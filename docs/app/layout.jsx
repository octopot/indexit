import { Footer, Layout, Navbar } from 'nextra-theme-docs'
import { Head } from 'nextra/components'
import { getPageMap } from 'nextra/page-map'
import 'nextra-theme-docs/style.css'

export const metadata = { title: 'Indexit', description: 'Indexit documentation' }

function withExportLinks(items) {
  return items.map(item => ({
    ...item,
    ...('frontMatter' in item && {
      href: item.href || `${item.route.replace(/\/$/, '')}/`,
    }),
    ...(item.children && { children: withExportLinks(item.children) }),
  }))
}

export default async function RootLayout({ children }) {
  return (
    <html lang="en" dir="ltr" suppressHydrationWarning>
      <Head />
      <body>
        <Layout
          navbar={
            <Navbar
              logo={<b>Indexit</b>}
              projectLink="https://github.com/octopot/indexit"
            />
          }
          pageMap={withExportLinks(await getPageMap())}
          docsRepositoryBase="https://github.com/octopot/indexit/tree/main/docs"
          footer={
            <Footer>
              MIT {new Date().getFullYear()} © <a href="https://github.com/octolab">OctoLab</a>.
            </Footer>
          }
          search={null}
        >
          {children}
        </Layout>
        <style>{'main a img { display: inline; }'}</style>
      </body>
    </html>
  )
}
