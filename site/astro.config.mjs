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
						alternateName: 'Lynx process manager',
						description:
							'Systemd-native process manager for Linux. Compiled Go daemon — 15 MB idle, 8 ms cold start, DynamicUser + landlock sandboxing, zero-privilege deploy.',
						applicationCategory: 'DeveloperApplication',
						operatingSystem: 'Linux',
						softwareVersion: '0.9.8',
						keywords: 'process manager, Linux, systemd, PM2 alternative, supervisor alternative, Go, daemon',
						offers: { '@type': 'Offer', price: '0' },
						downloadUrl: 'https://github.com/Jaro-c/Lynx/releases',
						url: 'https://jaro-c.github.io/Lynx/',
						sameAs: ['https://github.com/Jaro-c/Lynx'],
					}),
				},
				{
					tag: 'script',
					attrs: { type: 'application/ld+json' },
					content: JSON.stringify({
						'@context': 'https://schema.org',
						'@type': 'Organization',
						name: 'Lynx',
						url: 'https://jaro-c.github.io/Lynx/',
						sameAs: ['https://github.com/Jaro-c/Lynx'],
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
						{ label: 'Lynx vs PM2', slug: 'guides/vs-pm2' },
						{ label: 'Lynx vs Supervisor', slug: 'guides/vs-supervisor' },
						{ label: 'systemd-native', slug: 'guides/systemd-process-manager' },
						{ label: 'Lightweight & fast', slug: 'guides/lightweight-process-manager' },
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
