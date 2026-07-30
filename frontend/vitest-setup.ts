// jsdom (as of v27) no longer stubs the deprecated document.execCommand API at
// all — the property doesn't exist, so `vi.spyOn(document, 'execCommand')`
// fails with "property not defined on the object" even though real browsers
// (including the ones this clipboard fallback targets) still implement it.
// Define a no-op stub so tests can spy on/override it like they would in a
// real browser.
if (typeof document !== 'undefined' && !('execCommand' in document)) {
	// @ts-expect-error -- jsdom doesn't type this property at all
	document.execCommand = () => false;
}
