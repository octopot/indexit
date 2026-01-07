import nextra from 'nextra'

const withNextra = nextra({})
const staticExport = process.env.TARGET === 'static'

export default withNextra({
  basePath: process.env.BASE_PATH || '',
  trailingSlash: true,
  // Keep explicit slashes on version-like routes such as /changelog/v1.0.0/.
  skipTrailingSlashRedirect: true,
  ...(staticExport && {
    output: 'export',
    distDir: 'dist',
    images: { unoptimized: true },
  }),
})
