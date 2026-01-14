export default {
  '*': { theme: { timestamp: false } },
  index: {
    title: 'Home', type: 'page', display: 'hidden',
    theme: { layout: 'full', sidebar: false, toc: false, breadcrumb: false, pagination: false, copyPage: false },
  },
  documentation: { title: 'Documentation', type: 'page', href: '/quick-start/' },
  'quick-start': 'Quick start',
  guide: { title: 'Documentation', display: 'children' },
  changelog: { title: 'Changelog', type: 'page', theme: { pagination: false } },
}
