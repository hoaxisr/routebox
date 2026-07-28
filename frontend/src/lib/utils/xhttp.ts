/**
 * What the panel writes into every xhttp transport it builds.
 *
 * The field looks optional and is not. The fork declares `x_padding_bytes`
 * without omitempty as a plain Range, so an absent field decodes to {0,0}, and
 * its check then rejects From<=0||To<=0 with "x_padding_bytes cannot be
 * disabled" — refusing to load the ENTIRE config, inbound or outbound alike.
 * (There is a normalizer that would default exactly this range, but it runs
 * after the check.) Mirrors config.XHTTPDefaultPadding on the Go side.
 */
export const XHTTP_DEFAULT_PADDING = '100-1000';
