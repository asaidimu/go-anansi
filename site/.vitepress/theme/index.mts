// Custom VitePress theme — extends the default theme with site tokens.

import DefaultTheme from 'vitepress/theme'
import type { Theme } from 'vitepress'
import './styles/custom.css'

export default {
  extends: DefaultTheme,
  enhanceApp({ app, router, siteData }) {
    // Future: register custom Vue components for callouts, phase pickers, etc.
  },
} satisfies Theme
