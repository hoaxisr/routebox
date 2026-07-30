// Copy text to the clipboard, falling back to a hidden textarea + execCommand.
// navigator.clipboard is usable only in a secure context: fine on a VPS behind
// HTTPS, not in router mode where the panel serves plain HTTP on a LAN address.
// Returns whether the copy succeeded; never throws.
export async function copyText(text: string): Promise<boolean> {
	try {
		if (navigator.clipboard && window.isSecureContext) {
			await navigator.clipboard.writeText(text);
			return true;
		}
		const ta = document.createElement('textarea');
		ta.value = text;
		ta.style.position = 'fixed';
		ta.style.opacity = '0';
		document.body.appendChild(ta);
		ta.select();
		const ok = document.execCommand('copy');
		document.body.removeChild(ta);
		return ok;
	} catch {
		return false;
	}
}
