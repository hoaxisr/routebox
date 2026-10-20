// Reverse-proxy snippets for the Telegram WEB front on installs where RouteBox
// does not own the public web server (not all-in-one). The operator pastes one
// into their own nginx/Caddy, so it has to work verbatim — except for nginx's
// certificate paths, which only the operator knows. Those are placeholder
// lines on certbot's default layout, marked as such inside the snippet.

// A wildcard bind is valid for listening but not for dialling: nginx and Caddy
// would be told to connect to "0.0.0.0", which is not what anyone means. The
// front is reached over loopback from the same host.
function upstream(listen: string): string {
	if (listen.startsWith('0.0.0.0:')) return '127.0.0.1:' + listen.slice('0.0.0.0:'.length);
	if (listen.startsWith('[::]:')) return '127.0.0.1:' + listen.slice('[::]:'.length);
	return listen;
}

export function nginxSnippet(hostname: string, listen: string): string {
	return [
		'server {',
		'\tlisten 443 ssl;',
		`\tserver_name ${hostname};`,
		'',
		"\t# Placeholders: point these at this name's certificate, or drop them",
		'\t# if your existing server block already sets them.',
		`\tssl_certificate /etc/letsencrypt/live/${hostname}/fullchain.pem;`,
		`\tssl_certificate_key /etc/letsencrypt/live/${hostname}/privkey.pem;`,
		'',
		'\tlocation / {',
		`\t\tproxy_pass http://${upstream(listen)};`,
		'\t\tproxy_set_header Host $host;',
		'\t\tproxy_set_header X-Forwarded-For $remote_addr;',
		'\t\tproxy_read_timeout 60s;',
		'\t\tclient_max_body_size 2m;',
		'\t\tproxy_buffering off;',
		'\t}',
		'}'
	].join('\n');
}

export function caddySnippet(hostname: string, listen: string): string {
	return `${hostname} {\n\treverse_proxy ${upstream(listen)}\n}`;
}
