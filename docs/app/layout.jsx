import { Footer, Layout, Navbar } from 'nextra-theme-docs'
import { Head } from 'nextra/components'
import { getPageMap } from 'nextra/page-map'
import 'nextra-theme-docs/style.css'
import './globals.css'
import { siteUrl } from '../site.mjs'

export const metadata = {
  title: { default: 'indexit — Your Telegram. Your data.', template: '%s · indexit' },
  description: 'Export conversations as JSONL, save photos, and put your Telegram data to work.',
  metadataBase: new URL(siteUrl),
  icons: { icon: `${process.env.BASE_PATH || ''}/icon.svg` },
}

function withExportLinks(items) {
  return items.map(item => ({
    ...item,
    ...('frontMatter' in item && { href: item.href || `${item.route.replace(/\/$/, '')}/` }),
    ...(item.children && { children: withExportLinks(item.children) }),
  }))
}

export default async function RootLayout({ children }) {
  return (
    <html lang="en" dir="ltr" suppressHydrationWarning>
      <Head color={{ hue: 164, saturation: 62, lightness: { light: 30, dark: 66 } }} backgroundColor={{ light: 'rgb(250, 249, 246)', dark: 'rgb(20, 24, 23)' }} />
      <body>
        <Layout
          navbar={<Navbar logo={<span className="wordmark"><span className="brand-mark" aria-hidden="true">i<span>↗</span></span>indexit<span className="nav-version">v0.1.0</span></span>} projectLink="https://github.com/octopot/indexit" />}
          pageMap={withExportLinks(await getPageMap())}
          docsRepositoryBase="https://github.com/octopot/indexit/tree/main/docs"
          footer={<Footer><div className="site-footer"><span><b>indexit</b> <span>It’s all indexed.</span></span><span>MIT © {new Date().getFullYear()} <a href="https://github.com/octolab">OctoLab</a></span></div></Footer>}
          search={null}
          copyPageButton={false}
          feedback={{ content: 'Something unclear?', link: 'https://github.com/octopot/indexit/issues/new' }}
          nextThemes={{ defaultTheme: 'system' }}
        >
          {children}
        </Layout>
      </body>
    </html>
  )
}
