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
						{
							label: 'Comparisons',
							collapsed: false,
							items: [
								{ label: 'What is a process manager?', slug: 'guides/what-is-a-process-manager' },
								{ label: 'Lynx vs PM2', slug: 'guides/vs-pm2' },
								{ label: 'Lynx vs Supervisor', slug: 'guides/vs-supervisor' },
								{ label: 'PM2 vs Supervisor vs Lynx', slug: 'guides/pm2-vs-supervisor-vs-lynx' },
							],
						},
						{
							label: 'Architecture',
							collapsed: false,
							items: [
								{ label: 'systemd-native process manager', slug: 'guides/systemd-process-manager' },
								{ label: 'Lightweight & fast', slug: 'guides/lightweight-process-manager' },
								{ label: 'DynamicUser sandboxing', slug: 'guides/systemd-dynamicuser' },
							],
						},
						{
							label: 'How-to',
							collapsed: false,
							items: [
								{ label: 'Node.js as a Linux service', slug: 'guides/nodejs-linux-service' },
								{ label: 'Python worker as a Linux service', slug: 'guides/python-worker-linux' },
								{ label: 'Go binary as a systemd service', slug: 'guides/go-binary-systemd-service' },
								{ label: 'Multiple Node.js apps on a VPS', slug: 'guides/manage-multiple-nodejs-apps-vps' },
								{ label: 'Auto-restart on crash', slug: 'guides/auto-restart-on-crash' },
								{ label: 'Zero-downtime deployment', slug: 'guides/zero-downtime-deployment-linux' },
								{ label: 'Environment variables', slug: 'guides/linux-service-environment-variables' },
								{ label: 'Monitor memory & CPU', slug: 'guides/monitor-process-memory-cpu-linux' },
								{ label: 'Cron job management', slug: 'guides/linux-cron-job-management' },
							],
						},
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
