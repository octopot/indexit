export default {
  '*': { theme: { timestamp: false } },
  index: {
    title: 'Home', type: 'page', display: 'hidden',
    theme: { layout: 'full', sidebar: false, toc: false, breadcrumb: false, pagination: false, copyPage: false },
  },
  guide: { title: 'Documentation', type: 'page' },
  changelog: { title: 'Changelog', type: 'page', theme: { pagination: false } },
}
