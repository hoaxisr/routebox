import { describe, it, expect } from 'vitest';
import en from './locales/en.json';
import ru from './locales/ru.json';

// Keys of the Telegram proxy page. Both locales must carry every one: a missing
// key renders the raw dotted path, and this page hands out credentials and
// warns about invalidating them — not a place for "telegram.rotateConfirm" to
// show up as literal text.
const REQUIRED_KEYS = [
	'nav.telegram',
	// Outside the telegram block, so nothing else pins them. These landed in
	// the wrong block once already, which is what this list is for.
	'logs.sourceSingbox',
	'logs.sourceMtproto',
	'monitor.usageMtprotoClient',
	'telegram.title',
	'telegram.description',
	'telegram.loadFailed',
	'telegram.saveFailed',
	'telegram.running',
	'telegram.stopped',
	'telegram.enable',
	'telegram.enabling',
	'telegram.disable',
	'telegram.listenPort',
	'telegram.connected',
	'telegram.clientsLabel',
	'telegram.publicAddress',
	'telegram.clients',
	'telegram.addClient',
	'telegram.adding',
	'telegram.newClientPlaceholder',
	'telegram.emptyTitle',
	'telegram.emptyDesc',
	'telegram.online',
	'telegram.offline',
	'telegram.disabled',
	'telegram.expiredLabel',
	'telegram.noExpiry',
	'telegram.expiresIn',
	'telegram.setExpiry',
	'telegram.renew',
	'telegram.neverExpiry',
	'telegram.setDate',
	'telegram.save',
	'telegram.cancel',
	'telegram.share',
	'telegram.qr',
	'telegram.copyLink',
	'telegram.copied',
	'telegram.rotate',
	'telegram.rotateConfirm',
	'telegram.rotated',
	'telegram.delete',
	'telegram.deleteConfirm',
	'telegram.deleted',
	'telegram.added',
	'telegram.enabledClient',
	'telegram.disabledClient',
	'telegram.linkBlocked',
	'telegram.serverSettings',
	'telegram.serverSettingsSub',
	'telegram.groupListen',
	'telegram.groupLinks',
	'telegram.groupLinksNote',
	'telegram.groupBehaviour',
	'telegram.listen',
	'telegram.listenHint',
	'telegram.maskingDomain',
	'telegram.maskingDomainHint',
	'telegram.maskingDomainWarning',
	'telegram.publicHost',
	'telegram.publicHostHint',
	'telegram.publicPort',
	'telegram.publicPortHint',
	// listenPortField is its own key rather than a reuse of listenPort: that one
	// is the status strip's caption ("порт"), not a form label.
	'telegram.listenPortField',
	'telegram.publicPortPlaceholder',
	'telegram.concurrency',
	'telegram.idleTimeout',
	'telegram.preferIp',
	'telegram.preferIpHint',
	'telegram.preferIpv4',
	'telegram.preferIpv6',
	'telegram.onlyIpv4',
	'telegram.onlyIpv6',
	// The option list must not name the same choice twice: the empty value IS
	// mtglib's default (prefer-ipv6), so it is labelled as the default rather
	// than repeating that name (issue #62).
	'common.default',
	'telegram.outbound',
	'telegram.outboundHint',
	'telegram.outboundDirect',
	'telegram.outboundNone',
	'telegram.outboundMissing',
	'telegram.outboundMissingWarning',
	'telegram.socksPort',
	'telegram.socksPortHint',
	'telegram.settingsSaved',
	'telegram.connections',
	'telegram.noConnections',
	'telegram.connClient',
	'telegram.connIp',
	'telegram.connStarted',
	'telegram.needsClient',
	'telegram.needsDomain',
	'telegram.startFailed',
	'telegram.readOnly'
];

// The placeholder a message interpolates has to exist in both locales, or one
// language silently renders the literal token.
const REQUIRED_PLACEHOLDERS: Record<string, string[]> = {
	'telegram.expiresIn': ['rel'],
	'telegram.rotateConfirm': ['name'],
	'telegram.deleteConfirm': ['name']
};

function resolve(obj: unknown, path: string): unknown {
	return path.split('.').reduce<unknown>((acc, k) => (acc as Record<string, unknown>)?.[k], obj);
}

describe('telegram i18n keys', () => {
	for (const [name, dict] of [
		['en', en],
		['ru', ru]
	] as const) {
		for (const key of REQUIRED_KEYS) {
			it(`${name} has ${key}`, () => {
				const value = resolve(dict, key);
				expect(typeof value).toBe('string');
				expect(value).not.toBe('');
			});
		}

		for (const [key, placeholders] of Object.entries(REQUIRED_PLACEHOLDERS)) {
			it(`${name} keeps the placeholders in ${key}`, () => {
				const value = resolve(dict, key) as string;
				for (const p of placeholders) {
					expect(value).toContain(`{${p}}`);
				}
			});
		}

		// The IP-preference select listed "prefer-ipv6" twice, because the empty
		// option was labelled with the value it defaults to instead of being
		// called the default. Two identical rows in a radio list is not a choice
		// anyone can make (issue #62).
		it(`${name} gives every IP-preference option a distinct label`, () => {
			const labels = [
				'common.default',
				'telegram.preferIpv4',
				'telegram.preferIpv6',
				'telegram.onlyIpv4',
				'telegram.onlyIpv6'
			].map((k) => resolve(dict, k) as string);

			expect(new Set(labels).size).toBe(labels.length);
		});

		// Three sections offer the same action and used two different words for
		// it; the odd one out was the Telegram page (issue #62).
		it(`${name} labels the expiry date button the way the rest of the panel does`, () => {
			expect(resolve(dict, 'telegram.setDate')).toBe(resolve(dict, 'users.setDate'));
		});
	}
});
