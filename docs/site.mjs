// The public origin of the site, derived at build time: cd.docs.yml passes the
// GitHub Pages URL as SITE_URL, so a domain change needs no edits here.
export const siteUrl = (process.env.SITE_URL || 'http://localhost:3000').replace(/\/*$/, '/')
