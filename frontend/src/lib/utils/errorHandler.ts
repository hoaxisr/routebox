/**
 * Error handling utilities
 */

import { notifications } from '$lib/stores';

/**
 * Format unknown error to readable string
 */
export function formatError(err: unknown): string {
	if (err instanceof Error) {
		return err.message;
	}
	if (typeof err === 'string') {
		return err;
	}
	if (err && typeof err === 'object') {
		// Handle API error response objects
		if ('message' in err && typeof (err as { message: unknown }).message === 'string') {
			return (err as { message: string }).message;
		}
		if ('error' in err && typeof (err as { error: unknown }).error === 'string') {
			return (err as { error: string }).error;
		}
		// Try to stringify
		try {
			const str = JSON.stringify(err);
			if (str !== '{}') return str;
		} catch {
			// Ignore stringify errors
		}
	}
	return 'An unexpected error occurred';
}

/**
 * Handle API error with notification
 */
export function handleApiError(err: unknown, context: string): void {
	const message = formatError(err);
	console.error(`${context}:`, err);
	notifications.error(`${context}: ${message}`);
}

/**
 * Handle validation error with notification
 */
export function handleValidationError(field: string, message: string): void {
	notifications.error(`${field}: ${message}`);
}

/**
 * Try-catch wrapper for async operations
 * Returns null on error (after showing notification)
 */
export async function safeAsync<T>(
	operation: () => Promise<T>,
	errorContext: string
): Promise<T | null> {
	try {
		return await operation();
	} catch (err) {
		handleApiError(err, errorContext);
		return null;
	}
}

/**
 * Try-catch wrapper that returns a result object
 */
export async function tryAsync<T>(
	operation: () => Promise<T>
): Promise<{ ok: true; data: T } | { ok: false; error: string }> {
	try {
		const data = await operation();
		return { ok: true, data };
	} catch (err) {
		return { ok: false, error: formatError(err) };
	}
}

/**
 * Log error to console without notification
 * Use for non-critical errors that shouldn't interrupt user
 */
export function logError(err: unknown, context: string): void {
	console.error(`${context}:`, err);
}

/**
 * Create a formatted error message for form validation
 */
export function createValidationErrors(
	errors: Record<string, string | undefined>
): Record<string, string> {
	const result: Record<string, string> = {};
	for (const [key, value] of Object.entries(errors)) {
		if (value) {
			result[key] = value;
		}
	}
	return result;
}

/**
 * Check if there are any validation errors
 */
export function hasValidationErrors(errors: Record<string, string | undefined>): boolean {
	return Object.values(errors).some((v) => v !== undefined && v !== '');
}

/**
 * Clear all validation errors
 */
export function clearValidationErrors(errors: Record<string, string>): Record<string, string> {
	const result: Record<string, string> = {};
	for (const key of Object.keys(errors)) {
		result[key] = '';
	}
	return result;
}
