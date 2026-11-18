import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'pic-bridge',
  description: 'Import photos and videos from camera SD cards, verified.',
  base: '/pic-bridge/',
  themeConfig: {
    nav: [{ text: 'Guide', link: '/guide/install' }],
    sidebar: [
      {
        text: 'Guide',
        items: [
          { text: 'Installation', link: '/guide/install' },
          { text: 'Configuration', link: '/guide/configuration' },
          { text: 'Usage', link: '/guide/usage' },
        ],
      },
    ],
    search: { provider: 'local' },
    socialLinks: [
      { icon: 'github', link: 'https://github.com/tarciosaraiva/pic-bridge' },
    ],
  },
})
