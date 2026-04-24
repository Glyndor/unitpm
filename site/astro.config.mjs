// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
	site: 'https://jaro-c.github.io',
	base: '/Lynx',
	trailingSlash: 'ignore',
	integrations: [
		starlight({
			title: 'Lynx',
			description:
				'The secure, systemd-native process manager for Linux. A lean, hardened alternative to PM2 and Supervisor.',
			logo: {
				src: './src/assets/lynx.svg',
				replacesTitle: false,
			},
			favicon: '/favicon.svg',
			social: [
				{
					icon: 'github',
					label: 'GitHub',
					href: 'https://github.com/Jaro-c/Lynx',
				},
			],
			editLink: {
				baseUrl: 'https://github.com/Jaro-c/Lynx/edit/main/site/',
			},
			customCss: ['./src/styles/custom.css'],
			head: [
				{
					tag: 'meta',
					attrs: {
						property: 'og:image',
						content: 'https://jaro-c.github.io/Lynx/og.png',
					},
				},
				{
					tag: 'meta',
					attrs: {
						name: 'twitter:card',
						content: 'summary_large_image',
					},
				},
				{
					tag: 'meta',
					attrs: {
						name: 'twitter:image',
						content: 'https://jaro-c.github.io/Lynx/og.png',
					},
				},
				{
					tag: 'script',
					attrs: { type: 'application/ld+json' },
					content: JSON.stringify({
						'@context': 'https://schema.org',
						'@type': 'SoftwareApplication',
						name: 'Lynx',
						description:
							'The secure, systemd-native process manager for Linux. A lean, hardened alternative to PM2 and Supervisor.',
						applicationCategory: 'DeveloperApplication',
						operatingSystem: 'Linux',
						offers: { '@type': 'Offer', price: '0' },
						url: 'https://jaro-c.github.io/Lynx/',
					}),
				},
			],
			sidebar: [
				{
					label: 'Getting started',
					items: [
						{ label: 'Introduction', slug: 'start/introduction' },
						{ label: 'Install', slug: 'start/install' },
						{ label: 'Quickstart', slug: 'start/quickstart' },
						{ label: 'Access model', slug: 'start/access-model' },
					],
				},
				{
					label: 'Guides',
					items: [
						{ label: 'Runtimes', slug: 'guides/runtimes' },
						{ label: 'Tutorials', slug: 'guides/tutorials' },
						{ label: 'FAQ', slug: 'guides/faq' },
					],
				},
				{
					label: 'Reference',
					items: [
						{ label: 'Architecture', slug: 'reference/architecture' },
						{ label: 'Security', slug: 'reference/security' },
						{
							label: 'Commands',
							autogenerate: { directory: 'reference/commands' },
						},
					],
				},
			],
		}),
	],
});
