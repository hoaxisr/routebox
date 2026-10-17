import { describe, it, expect } from 'vitest';
import { nginxSnippet, caddySnippet } from './webSnippet';

describe('nginxSnippet', () => {
	it('carries every directive the web front needs', () => {
		const s = nginxSnippet('proxy.example.com', '127.0.0.1:9446');
		for (const line of [
			'server_name proxy.example.com;',
			'ssl_certificate /etc/letsencrypt/live/proxy.example.com/fullchain.pem;',
			'ssl_certificate_key /etc/letsencrypt/live/proxy.example.com/privkey.pem;',
			'proxy_pass http://127.0.0.1:9446;',
			'proxy_set_header Host $host;',
			'proxy_set_header X-Forwarded-For $remote_addr;',
			'proxy_read_timeout 60s;',
			'client_max_body_size 2m;',
			'proxy_buffering off;'
		]) {
			expect(s).toContain(line);
		}
	});

	it('marks the certificate paths as placeholders', () => {
		const s = nginxSnippet('proxy.example.com', '127.0.0.1:9446');
		expect(s).toMatch(/# Placeholders:.*\n.*\n\tssl_certificate /);
	});

	it('points a wildcard listen address at loopback', () => {
		expect(nginxSnippet('h', '0.0.0.0:9446')).toContain('proxy_pass http://127.0.0.1:9446;');
		expect(nginxSnippet('h', '[::]:9446')).toContain('proxy_pass http://127.0.0.1:9446;');
	});
});

describe('caddySnippet', () => {
	it('is the three-line reverse_proxy block', () => {
		expect(caddySnippet('h', '127.0.0.1:9446')).toBe('h {\n\treverse_proxy 127.0.0.1:9446\n}');
	});

	it('points a wildcard listen address at loopback', () => {
		expect(caddySnippet('h', '0.0.0.0:9446')).toContain('reverse_proxy 127.0.0.1:9446');
	});
});
