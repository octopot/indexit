#!/usr/bin/env node
// Release guardrails shared by the pre-push hook, make targets and CI.
// Zero dependencies: Node and git only.
//
//   release.mjs check  <tag> [--sha <commit>] [--pushed <branch>=<sha>]...
//   release.mjs render <tag> --site-url <url> --out <file>   (prints the title)
//   release.mjs preflight                                    (CI: required secrets are present and usable)
//   release.mjs doctor                                       (on demand: config vs GitHub, with remediation)

import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync, writeFileSync } from 'node:fs'

const SETTINGS = '.github/settings.json'
const DEFAULTS = {
  tag_pattern: '^v\\d+\\.\\d+\\.\\d+(-[0-9A-Za-z.-]+)?$',
  notes: 'docs/content/changelog/{tag}.md',
  branches: [],
}

// --- helpers -----------------------------------------------------------------

function git(...args) {
  return execFileSync('git', args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] })
}

function tryGit(...args) {
  try { return git(...args) } catch { return null }
}

function gh(...args) {
  const env = typeof args.at(-1) === 'object' ? { ...process.env, ...args.pop() } : process.env
  try {
    return execFileSync('gh', args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'], env })
  } catch (e) {
    const err = new Error((e.stderr || e.message || '').trim())
    err.status = e.status
    throw err
  }
}

function settings(sha) {
  const raw = sha ? tryGit('show', `${sha}:${SETTINGS}`) : (existsSync(SETTINGS) ? readFileSync(SETTINGS, 'utf8') : null)
  const cfg = raw ? JSON.parse(raw) : {}
  return { release: { ...DEFAULTS, ...(cfg['x-release'] || {}) }, secrets: cfg.secrets || {} }
}

function args(argv) {
  const out = { _: [], pushed: {} }
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]
    if (a === '--pushed') {
      const [b, s] = argv[++i].split('=')
      out.pushed[b] = s
    } else if (a.startsWith('--')) {
      out[a.slice(2)] = argv[++i]
    } else {
      out._.push(a)
    }
  }
  return out
}

// Splits a Markdown document into frontmatter and body, parsing `key: value` lines.
function frontmatter(text) {
  const m = text.match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n/)
  if (!m) return { meta: null, body: text }
  const meta = {}
  for (const line of m[1].split(/\r?\n/)) {
    const kv = line.match(/^([A-Za-z_][\w-]*):\s*(.*)$/)
    if (!kv) continue
    let v = kv[2].trim()
    if (/^".*"$/.test(v)) v = JSON.parse(v)
    else if (/^'.*'$/.test(v)) v = v.slice(1, -1).replace(/''/g, "'")
    meta[kv[1]] = v
  }
  return { meta, body: text.slice(m[0].length) }
}

// Calls fn for every piece of prose, skipping fenced code blocks and inline code spans.
function mapProse(body, fn) {
  let fence = null
  return body.split('\n').map((line, i) => {
    const f = line.match(/^\s{0,3}(`{3,}|~{3,})/)
    if (fence) {
      if (f && f[1][0] === fence[0] && f[1].length >= fence.length) fence = null
      return line
    }
    if (f) { fence = f[1]; return line }
    return line.split(/(`+[^`]*`+)/).map((part, j) => (j % 2 ? part : fn(part, i + 1))).join('')
  }).join('\n')
}

function releaseBranch(tag, release, defaultBranch) {
  for (const { match, branch } of release.branches) {
    const m = tag.match(new RegExp(match))
    if (m) return branch.replace(/\$(\d+)/g, (_, n) => m[n] ?? '')
  }
  return defaultBranch
}

function defaultBranch() {
  if (process.env.DEFAULT_BRANCH) return process.env.DEFAULT_BRANCH
  const head = tryGit('symbolic-ref', '--short', 'refs/remotes/origin/HEAD')
  return head ? head.trim().replace(/^origin\//, '') : null
}

// --- check -------------------------------------------------------------------

function check(tag, opts) {
  const errors = []
  const sha = (opts.sha || tryGit('rev-parse', `${tag}^{commit}`) || '').trim()
  if (!sha) return [`${tag}: tag not found; create it first`]

  const { release } = settings(sha)
  if (!new RegExp(release.tag_pattern).test(tag)) {
    errors.push(`${tag}: does not match x-release.tag_pattern ${release.tag_pattern} in ${SETTINGS}`)
  }

  const note = release.notes.replace('{tag}', tag)
  const committed = tryGit('show', `${sha}:${note}`)
  if (committed === null) {
    errors.push(`${note}: missing in ${tag}; write the release note, commit it and move the tag`)
  } else {
    const staged = tryGit('show', `:${note}`)
    const worktree = existsSync(note) ? readFileSync(note, 'utf8') : null
    if (staged !== committed || worktree !== committed) {
      errors.push(`${note}: has changes not in ${tag}; commit them and move the tag`)
    }
    errors.push(...lint(note, committed))
  }

  const base = defaultBranch()
  const branch = releaseBranch(tag, release, base)
  if (!branch) {
    errors.push(`${tag}: cannot resolve the default branch; run: git remote set-head origin --auto`)
  } else {
    const tip = opts.pushed[branch] || tryGit('rev-parse', '--verify', '-q', `refs/remotes/origin/${branch}`)?.trim()
    if (!tip) {
      errors.push(`${tag}: branch ${branch} is unknown; push it together with the tag: git push --atomic origin ${branch} ${tag}`)
    } else if (tryGit('merge-base', '--is-ancestor', sha, tip) === null) {
      errors.push(`${tag}: its commit is not on ${branch}; push both at once: git push --atomic origin ${branch} ${tag}`)
    }
  }
  return errors
}

function lint(file, text) {
  const errors = []
  const { meta, body } = frontmatter(text)
  if (!meta) return [`${file}: no frontmatter; add title and description`]
  for (const key of ['title', 'description']) {
    if (!meta[key]) errors.push(`${file}: frontmatter has no ${key}`)
  }
  const first = body.split('\n').find((l) => l.trim() !== '')
  if (!first || first.trim() !== `# ${meta.title}`) {
    errors.push(`${file}: the first line must be the H1 "# ${meta.title}", as in frontmatter title`)
  }
  const offset = text.slice(0, text.length - body.length).split('\n').length - 1
  let h1 = 0
  mapProse(body, (prose, n) => {
    const line = n + offset
    if (/^# /.test(prose)) h1++
    if (/^\s*(import|export)\s/.test(prose)) errors.push(`${file}:${line}: MDX import/export is not supported in release notes`)
    if (/<[A-Z][\w.]*[\s/>]/.test(prose)) errors.push(`${file}:${line}: MDX components are not supported in release notes`)
    return prose
  })
  if (h1 > 1) errors.push(`${file}: more than one H1`)
  return errors
}

// --- render ------------------------------------------------------------------

function render(tag, opts) {
  if (!opts['site-url'] || !opts.out) throw new Error('render needs --site-url and --out')
  const { release } = settings()
  const note = release.notes.replace('{tag}', tag)
  const text = readFileSync(note, 'utf8')
  const errors = lint(note, text)
  if (errors.length) throw new Error(errors.join('\n'))

  const { meta, body } = frontmatter(text)
  const site = opts['site-url'].replace(/\/+$/, '')
  const abs = (url) => `${site}${url}`
  const out = mapProse(body.replace(/^\s*# .*\n+/, ''), (prose) => prose
    .replace(/(\]\()(\/(?!\/)[^)\s]*)/g, (_, p, url) => p + abs(url))           // [text](/path) and ![alt](/path)
    .replace(/^(\s*\[[^\]]+\]:\s*)(\/(?!\/)\S*)/, (_, p, url) => p + abs(url)) // [ref]: /path
    .replace(/(\s(?:href|src)=["'])(\/(?!\/)[^"']*)/g, (_, p, url) => p + abs(url)))
  writeFileSync(opts.out, out.trimEnd() + '\n')
  process.stdout.write(meta.title + '\n')
}

// --- preflight and doctor ----------------------------------------------------

// Reads the first Homebrew repository block from .goreleaser.yml.
function tap() {
  const cfg = existsSync('.goreleaser.yml') ? readFileSync('.goreleaser.yml', 'utf8') : ''
  const block = cfg.match(/^(?:homebrew_casks|brews):[\s\S]*?repository:\s*\n((?:\s{6,}.*\n)+)/m)
  if (!block) return null
  const field = (k) => block[1].match(new RegExp(`^\\s+${k}:\\s*["']?([^"'\\n]+)`, 'm'))?.[1].trim()
  const token = field('token')?.match(/\.Env\.(\w+)/)?.[1]
  return { owner: field('owner'), name: field('name'), token }
}

function secretGuide(name, t) {
  if (t && t.token === name) {
    return `create a fine-grained PAT with resource owner ${t.owner}, repository ${t.owner}/${t.name}, ` +
      `permission Contents: Read and write; save it as secret ${name} ` +
      `(gh secret set ${name} -o <org> or -R <owner>/<repo>)`
  }
  return `save it as secret ${name} (gh secret set ${name} -o <org> or -R <owner>/<repo>)`
}

function preflight() {
  const t = tap()
  const errors = []
  if (t?.token) {
    const token = process.env[t.token]
    if (!token) {
      errors.push(`secret ${t.token} is empty or not passed to this step; ${secretGuide(t.token, t)}`)
    } else {
      try {
        gh('api', `repos/${t.owner}/${t.name}`, '--silent', { GH_TOKEN: token })
      } catch (e) {
        errors.push(`${t.token} cannot read ${t.owner}/${t.name} (${e.message}); ${secretGuide(t.token, t)}`)
      }
    }
  }
  return errors
}

function doctor() {
  const report = []
  const ok = (m) => report.push(['ok', m])
  const fail = (m) => report.push(['fail', m])
  const skip = (m) => report.push(['unverified', m])

  let repo
  try {
    repo = JSON.parse(gh('repo', 'view', '--json', 'nameWithOwner,defaultBranchRef'))
  } catch (e) {
    fail(`gh cannot reach the repository (${e.message}); run: gh auth login`)
    return report
  }
  const slug = repo.nameWithOwner
  const owner = slug.split('/')[0]

  const local = defaultBranch()
  const remote = repo.defaultBranchRef?.name
  if (local === remote) ok(`default branch ${remote}`)
  else fail(`local origin/HEAD is ${local ?? 'unset'}, GitHub says ${remote}; run: git remote set-head origin --auto`)

  try {
    const pages = JSON.parse(gh('api', `repos/${slug}/pages`))
    if (pages.build_type === 'workflow') ok(`Pages publishes ${pages.html_url} from GitHub Actions`)
    else fail(`Pages builds from a branch; set Settings → Pages → Source: GitHub Actions (https://github.com/${slug}/settings/pages)`)
  } catch {
    skip(`Pages is not enabled or not readable; enable it in https://github.com/${slug}/settings/pages if docs are published`)
  }

  const t = tap()
  const { secrets } = settings()
  const names = new Set([...Object.keys(secrets), ...(t?.token ? [t.token] : [])])
  const workflows = existsSync('.github/workflows')
    ? git('ls-files', '.github/workflows').trim().split('\n').filter(Boolean)
    : []
  const list = (flag, target) => {
    try { return new Set(JSON.parse(gh('secret', 'list', flag, target, '--json', 'name')).map((s) => s.name)) } catch { return null }
  }
  const repoSecrets = list('-R', slug)
  const orgSecrets = list('-o', owner)
  for (const name of names) {
    const used = workflows.filter((f) => readFileSync(f, 'utf8').includes(`secrets.${name}`))
    if (!used.length) fail(`secret ${name} is declared but no workflow passes it; reference \${{ secrets.${name} }} where it is needed`)
    if (repoSecrets?.has(name)) ok(`secret ${name} is set on ${slug}${used.length ? `, used by ${used.join(', ')}` : ''}`)
    else if (orgSecrets?.has(name)) ok(`secret ${name} is set on organization ${owner}${used.length ? `, used by ${used.join(', ')}` : ''}`)
    else if (orgSecrets === null) skip(`secret ${name} is not on ${slug}; organization secrets are not readable (gh auth refresh -s admin:org); if missing: ${secretGuide(name, t)}`)
    else fail(`secret ${name} is missing; ${secretGuide(name, t)}`)
  }

  // make tools installs into bin/<os>/<arch>, named as Go names them
  const arch = { x64: 'amd64', ia32: '386' }[process.arch] || process.arch
  const goreleaser = [`bin/${process.platform}/${arch}`, ...(process.env.PATH || '').split(':')]
  try {
    execFileSync('goreleaser', ['check'], { stdio: 'ignore', env: { ...process.env, PATH: goreleaser.join(':') } })
    ok('goreleaser check')
  } catch (e) {
    if (e.code === 'ENOENT') skip('goreleaser is not installed; run: make tools')
    else fail('goreleaser check failed; run it to see the details')
  }
  return report
}

// --- main --------------------------------------------------------------------

const [mode, ...rest] = process.argv.slice(2)
const opts = args(rest)
try {
  switch (mode) {
    case 'check': {
      const errors = check(opts._[0], opts)
      errors.forEach((e) => console.error(`release: ${e}`))
      process.exit(errors.length ? 1 : 0)
    }
    case 'render':
      render(opts._[0], opts)
      break
    case 'preflight': {
      const errors = preflight()
      errors.forEach((e) => console.error(`preflight: ${e}`))
      process.exit(errors.length ? 1 : 0)
    }
    case 'doctor': {
      const report = doctor()
      for (const [status, msg] of report) console.log(`${status.padEnd(10)} ${msg}`)
      process.exit(report.some(([s]) => s === 'fail') ? 1 : 0)
    }
    default:
      console.error('usage: release.mjs check|render|preflight|doctor ...')
      process.exit(2)
  }
} catch (e) {
  console.error(`release: ${e.message}`)
  process.exit(1)
}
