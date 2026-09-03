// Custom VitePress theme — extends the default theme and injects a
// global AlphaBanner above every page. The alpha state is ambient;
// it shouldn't be a footnote on page 1 only.

import DefaultTheme from 'vitepress/theme'
import type { Theme } from 'vitepress'
import AlphaBanner from './components/AlphaBanner.vue'
import './styles/custom.css'

export default {
  extends: DefaultTheme,
  Layout: () => {
    return h(DefaultTheme.Layout, null, {
      // Slot the banner into the top of the layout, above the nav bar.
      'layout-top': () => h(AlphaBanner),
    })
  },
  enhanceApp({ app, router, siteData }) {
    // Future: register custom Vue components for callouts, phase pickers, etc.
  },
} satisfies Theme

// h is a VitePress global; import it here to keep the theme file explicit.
import { h } from 'vue'
