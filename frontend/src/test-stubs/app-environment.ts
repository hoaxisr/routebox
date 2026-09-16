// Stub for SvelteKit's $app/environment, which only exists inside a Kit build.
// Mounting any component that touches a store (they read `browser` before going
// near localStorage) needs it; vitest.config.ts aliases $app/environment here.
export const browser = false;
export const dev = false;
export const building = false;
export const version = 'test';
