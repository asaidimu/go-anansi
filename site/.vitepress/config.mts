import { defineConfig } from 'vitepress'

// Go-Anansi docs site configuration.
// Sidebar grouping follows the phase-signal routing from
// skills/anansi/SKILL.md §7 — "Wire persistence → persistence-setup",
// "Cache it → caching", "RLS/audit/validate/encrypt → decorators".
// Diátaxis spine: Tutorial / Guides / Reference / Explanations
// plus RFCs (walled off), Examples, and Contribute.

export default defineConfig({
  lang: 'en-US',
  title: 'Go-Anansi',
  description:
    'A schema-driven, hybrid persistence layer for Go. Declare a schema, generate type-safe models, query through a hybrid engine, and ship with decorators + events.',

  // Site is served from /docs/ on GitHub Pages. Override with env var
  // for local dev or other hosts (e.g. Netlify custom domain).
  base: process.env.BASE_PATH || '/go-anansi/',

  cleanUrls: true,
  lastUpdated: true,

  head: [
    ['meta', { name: 'theme-color', content: '#b8860b' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:title', content: 'Go-Anansi' }],
    [
      'meta',
      {
        property: 'og:description',
        content:
          'A schema-driven, hybrid persistence layer for Go. Schemas are the single source of truth.',
      },
    ],
  ],

  markdown: {
    lineNumbers: false,
    toc: { level: [2, 3] },
    // Note: raw HTML is allowed in markdown by default in VitePress, so the
    // landing page's <div class="phase-picker"> works without extra plugins.
  },

  // Built-in local search — no Algolia account needed for an alpha project.
  themeConfig: {
    search: {
      provider: 'local',
      options: {
        translations: {
          button: { buttonText: 'Search docs', buttonAriaLabel: 'Search' },
          modal: {
            noResultsText: 'No results — try a different term.',
            displayDetails: 'Show details',
            resetButtonTitle: 'Clear query',
            backButtonTitle: 'Close search',
          },
        },
      },
    },

    siteTitle: 'Go-Anansi',
    // Logo is intentional — placeholder; drop a real SVG in site/public/logo.svg
    // logo: '/logo.svg',

    nav: [
      { text: 'Tutorial', link: '/tutorial/overview' },
      { text: 'Guides', link: '/guides/domain-modeling' },
      { text: 'Reference', link: '/reference/schema-format' },
      {
        text: 'More',
        items: [
          { text: 'Explanations', link: '/explanations/architecture' },
          { text: 'Examples', link: '/examples/' },
          { text: 'RFCs', link: '/rfc/' },
          { text: 'Contribute', link: '/contribute/getting-started' },
          { text: 'Changelog', link: '/changelog' },
        ],
      },
      {
        text: 'v8 (alpha)',
        items: [
          { text: 'GitHub', link: 'https://github.com/asaidimu/go-anansi' },
          { text: 'npm: @asaidimu/anansi', link: 'https://www.npmjs.com/package/@asaidimu/anansi' },
          { text: 'Report an issue', link: 'https://github.com/asaidimu/go-anansi/issues' },
        ],
      },
    ],

    sidebar: {
      '/tutorial/': [
        {
          text: 'Getting started',
          collapsed: false,
          items: [
            { text: 'Overview', link: '/tutorial/overview' },
            { text: 'Installation', link: '/tutorial/installation' },
            { text: 'Your first schema', link: '/tutorial/first-schema' },
            { text: 'Code generation basics', link: '/tutorial/codegen-basics' },
            { text: 'CRUD with generated models', link: '/tutorial/crud-with-models' },
            { text: 'Projections', link: '/tutorial/projections' },
            { text: 'Schema change workflow', link: '/tutorial/schema-change-workflow' },
          ],
        },
      ],

      '/guides/': [
        {
          text: 'Guides',
          collapsed: false,
          items: [
            { text: 'Domain modeling', link: '/guides/domain-modeling' },
            { text: 'Persistence setup', link: '/guides/persistence-setup' },
            { text: 'Transactions', link: '/guides/transactions' },
            { text: 'Caching', link: '/guides/caching' },
            { text: 'Decorators (RLS / audit / encrypt)', link: '/guides/decorators' },
            { text: 'Sanitization', link: '/guides/sanitization' },
            { text: 'Metadata providers', link: '/guides/metadata-providers' },
            { text: 'Observability', link: '/guides/observability' },
            { text: 'Events & subscriptions', link: '/guides/events-subscriptions' },
          ],
        },
      ],

      '/reference/': [
        {
          text: 'Reference',
          collapsed: false,
          items: [
            { text: 'Schema format', link: '/reference/schema-format' },
            { text: 'Query DSL', link: '/reference/query-dsl' },
            { text: 'Struct tag spec', link: '/reference/struct-tag-spec' },
            { text: 'Schema IR', link: '/reference/schema-ir' },
            { text: 'Schema rules', link: '/reference/schema-rules' },
            { text: 'Schema addressing', link: '/reference/schema-addressing' },
            { text: 'Schema versioning', link: '/reference/schema-versioning' },
            { text: 'Migration semantics', link: '/reference/migration-semantics' },
            { text: 'Codegen modes', link: '/reference/codegen-modes' },
            { text: 'Collection internals', link: '/reference/collection-internals' },
            { text: 'CLI & config', link: '/reference/cli' },
            { text: 'TypeScript package', link: '/reference/ts-package' },
          ],
        },
      ],

      '/explanations/': [
        {
          text: 'Explanations',
          collapsed: false,
          items: [
            { text: 'Architecture', link: '/explanations/architecture' },
            { text: 'Data flow', link: '/explanations/data-flow' },
            { text: 'Schema as source of truth', link: '/explanations/schema-as-source-of-truth' },
            { text: 'Hybrid query engine', link: '/explanations/hybrid-query-engine' },
            { text: 'Wire format (Go ⇄ TS)', link: '/explanations/wire-format' },
            { text: 'Licensing', link: '/explanations/licensing' },
          ],
        },
      ],

      '/examples/': [
        {
          text: 'Examples',
          collapsed: false,
          items: [
            { text: 'Catalog', link: '/examples/' },
            { text: 'Basic CRUD', link: '/examples/basic' },
            { text: 'REST API', link: '/examples/api' },
            { text: 'Events', link: '/examples/events' },
            { text: 'Migration', link: '/examples/migration' },
            { text: 'Complex schema', link: '/examples/complex' },
            { text: 'Benchmark', link: '/examples/benchmark' },
            { text: 'Go ⇄ TS round trip', link: '/examples/encoding-roundtrip' },
          ],
        },
      ],

      '/rfc/': [
        {
          text: 'RFCs',
          collapsed: false,
          items: [
            { text: 'RFC process & status', link: '/rfc/' },
            { text: 'Query language', link: '/rfc/query-language' },
            { text: 'Full-text search', link: '/rfc/search' },
            { text: 'Anansi encoding', link: '/rfc/anansi-encoding' },
            { text: 'Schema encoding', link: '/rfc/schema-encoding' },
            { text: 'BadgerDB interactor', link: '/rfc/badgerdb-interactor' },
            { text: 'Query engine overhaul', link: '/rfc/query-engine-overhaul' },
            { text: 'Monorepo decomposition', link: '/rfc/monorepo-decomposition' },
            { text: 'Container-backed document', link: '/rfc/container-backed-document-refactor' },
          ],
        },
      ],

      '/contribute/': [
        {
          text: 'Contribute',
          collapsed: false,
          items: [
            { text: 'Getting started', link: '/contribute/getting-started' },
            { text: 'Testing', link: '/contribute/testing' },
            { text: 'Code style', link: '/contribute/code-style' },
            { text: 'CLA', link: '/contribute/cla' },
            { text: 'Internals map', link: '/contribute/internals-map' },
          ],
        },
      ],
    },

    socialLinks: [
      { icon: 'github', link: 'https://github.com/asaidimu/go-anansi' },
    ],

    footer: {
      message: 'Released under the AGPLv3-or-later. Commercial license available.',
      copyright: 'Copyright © 2026 go-anansi contributors',
    },

    editLink: {
      pattern:
        'https://github.com/asaidimu/go-anansi/edit/main/site/:path',
      text: 'Edit this page on GitHub',
    },

    lastUpdatedText: 'Last updated',

    docFooter: {
      prev: 'Previous',
      next: 'Next',
    },

    outline: {
      level: [2, 3],
      label: 'On this page',
    },

    darkModeSwitchLabel: 'Theme',
    sidebarMenuLabel: 'Menu',
    returnToTopLabel: 'Back to top',

    externalLinkIcon: true,
  },
})
