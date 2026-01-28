/**
 * Validation utilities for form fields
 */

export interface ValidationResult {
	valid: boolean;
	error?: string;
}

/**
 * Validate port number (1-65535)
 */
export function validatePort(value: number | string | undefined | null): ValidationResult {
	if (value === undefined || value === null || value === '') {
		return { valid: false, error: 'Port is required' };
	}
	const port = typeof value === 'string' ? parseInt(value, 10) : value;
	if (isNaN(port)) {
		return { valid: false, error: 'Port must be a number' };
	}
	if (port < 1 || port > 65535) {
		return { valid: false, error: 'Port must be between 1 and 65535' };
	}
	return { valid: true };
}

/**
 * Validate required string field
 */
export function validateRequired(
	value: string | undefined | null,
	fieldName: string = 'Field'
): ValidationResult {
	if (!value || !value.trim()) {
		return { valid: false, error: `${fieldName} is required` };
	}
	return { valid: true };
}

/**
 * Validate tag uniqueness
 */
export function validateTag(
	tag: string | undefined | null,
	existingTags: string[] = [],
	currentTag?: string
): ValidationResult {
	if (!tag || !tag.trim()) {
		return { valid: false, error: 'Tag is required' };
	}
	const normalizedTag = tag.trim();
	const isDuplicate = existingTags.some((t) => t === normalizedTag && t !== currentTag);
	if (isDuplicate) {
		return { valid: false, error: 'Tag already exists' };
	}
	return { valid: true };
}

/**
 * Validate CIDR notation (IPv4/IPv6)
 */
export function validateCIDR(cidr: string | undefined | null): ValidationResult {
	if (!cidr || !cidr.trim()) {
		return { valid: false, error: 'CIDR is required' };
	}
	const value = cidr.trim();
	// IPv4 CIDR: 192.168.0.0/24
	const ipv4Pattern = /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/;
	// IPv6 CIDR: fd00::/8, 2001:db8::/32
	const ipv6Pattern = /^([0-9a-fA-F]{0,4}:){1,7}[0-9a-fA-F]{0,4}\/\d{1,3}$/;

	if (!ipv4Pattern.test(value) && !ipv6Pattern.test(value)) {
		return { valid: false, error: 'Invalid CIDR notation (e.g., 192.168.0.0/24 or fd00::/8)' };
	}
	return { valid: true };
}

/**
 * Validate Base64 string (for WireGuard keys - 44 chars)
 */
export function validateBase64Key(
	key: string | undefined | null,
	keyType: string = 'Key'
): ValidationResult {
	if (!key || !key.trim()) {
		return { valid: false, error: `${keyType} is required` };
	}
	// WireGuard keys are 32 bytes = 44 characters in base64 (with padding)
	const base64Pattern = /^[A-Za-z0-9+/]{43}=$/;
	if (!base64Pattern.test(key.trim())) {
		return { valid: false, error: `Invalid ${keyType} format (expected 44-char base64)` };
	}
	return { valid: true };
}

/**
 * Validate port range (e.g., "1000-2000" or "1000:2000")
 */
export function validatePortRange(range: string | undefined | null): ValidationResult {
	if (!range || !range.trim()) {
		return { valid: false, error: 'Port range is required' };
	}
	const value = range.trim();
	const pattern = /^\d{1,5}[-:]\d{1,5}$/;
	if (!pattern.test(value)) {
		return { valid: false, error: 'Invalid format (use: 1000-2000 or 1000:2000)' };
	}
	const [start, end] = value.split(/[-:]/).map(Number);
	if (start < 1 || end > 65535) {
		return { valid: false, error: 'Ports must be between 1 and 65535' };
	}
	if (start >= end) {
		return { valid: false, error: 'Start port must be less than end port' };
	}
	return { valid: true };
}

/**
 * Validate domain name
 */
export function validateDomain(domain: string | undefined | null): ValidationResult {
	if (!domain || !domain.trim()) {
		return { valid: false, error: 'Domain is required' };
	}
	const value = domain.trim().toLowerCase();
	// Basic domain pattern (allows subdomains, TLDs)
	const pattern =
		/^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z]{2,})?$/;
	if (!pattern.test(value)) {
		return { valid: false, error: 'Invalid domain format' };
	}
	return { valid: true };
}

/**
 * Validate UUID format
 */
export function validateUUID(uuid: string | undefined | null): ValidationResult {
	if (!uuid || !uuid.trim()) {
		return { valid: false, error: 'UUID is required' };
	}
	const pattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
	if (!pattern.test(uuid.trim())) {
		return { valid: false, error: 'Invalid UUID format' };
	}
	return { valid: true };
}

/**
 * Validate URL
 */
export function validateURL(url: string | undefined | null): ValidationResult {
	if (!url || !url.trim()) {
		return { valid: false, error: 'URL is required' };
	}
	try {
		new URL(url.trim());
		return { valid: true };
	} catch {
		return { valid: false, error: 'Invalid URL format' };
	}
}

/**
 * Validate IP address (IPv4 or IPv6)
 */
export function validateIP(ip: string | undefined | null): ValidationResult {
	if (!ip || !ip.trim()) {
		return { valid: false, error: 'IP address is required' };
	}
	const value = ip.trim();
	// IPv4
	const ipv4Pattern = /^(\d{1,3}\.){3}\d{1,3}$/;
	// IPv6 (simplified)
	const ipv6Pattern = /^([0-9a-fA-F]{0,4}:){1,7}[0-9a-fA-F]{0,4}$/;

	if (!ipv4Pattern.test(value) && !ipv6Pattern.test(value)) {
		return { valid: false, error: 'Invalid IP address format' };
	}
	return { valid: true };
}

/**
 * Validate non-empty array
 */
export function validateNonEmptyArray(
	arr: unknown[] | undefined | null,
	fieldName: string = 'Items'
): ValidationResult {
	if (!arr || arr.length === 0) {
		return { valid: false, error: `At least one ${fieldName.toLowerCase()} is required` };
	}
	return { valid: true };
}

/**
 * Validate optional port (allows empty, but validates if provided)
 */
export function validateOptionalPort(value: number | string | undefined | null): ValidationResult {
	if (value === undefined || value === null || value === '' || value === 0) {
		return { valid: true }; // Optional, so empty is OK
	}
	return validatePort(value);
}

/**
 * Validate positive integer
 */
export function validatePositiveInt(
	value: number | string | undefined | null,
	fieldName: string = 'Value'
): ValidationResult {
	if (value === undefined || value === null || value === '') {
		return { valid: false, error: `${fieldName} is required` };
	}
	const num = typeof value === 'string' ? parseInt(value, 10) : value;
	if (isNaN(num) || num < 1) {
		return { valid: false, error: `${fieldName} must be a positive integer` };
	}
	return { valid: true };
}
