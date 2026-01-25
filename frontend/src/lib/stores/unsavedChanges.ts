import { writable, get } from 'svelte/store';

export interface UnsavedChange {
	section: string;      // e.g., 'endpoints', 'outbounds', 'routes'
	description: string;  // e.g., 'Added endpoint "MyVPN"'
}

interface UnsavedChangesState {
	hasChanges: boolean;
	changes: UnsavedChange[];
	showConfirmDialog: boolean;
	pendingNavigation: string | null;
}

function createUnsavedChangesStore() {
	const { subscribe, set, update } = writable<UnsavedChangesState>({
		hasChanges: false,
		changes: [],
		showConfirmDialog: false,
		pendingNavigation: null
	});

	return {
		subscribe,

		// Mark that changes have been made
		markChanged: (section: string, description: string) => {
			update(state => ({
				...state,
				hasChanges: true,
				changes: [...state.changes.filter(c => c.description !== description), { section, description }]
			}));
		},

		// Clear all unsaved changes (after save or discard)
		clearChanges: () => {
			set({
				hasChanges: false,
				changes: [],
				showConfirmDialog: false,
				pendingNavigation: null
			});
		},

		// Show confirmation dialog when trying to navigate away
		requestNavigation: (path: string) => {
			const state = get({ subscribe });
			if (state.hasChanges) {
				update(s => ({
					...s,
					showConfirmDialog: true,
					pendingNavigation: path
				}));
				return false; // Block navigation
			}
			return true; // Allow navigation
		},

		// Cancel pending navigation
		cancelNavigation: () => {
			update(state => ({
				...state,
				showConfirmDialog: false,
				pendingNavigation: null
			}));
		},

		// Confirm discard and navigate
		confirmDiscard: () => {
			const state = get({ subscribe });
			const path = state.pendingNavigation;
			set({
				hasChanges: false,
				changes: [],
				showConfirmDialog: false,
				pendingNavigation: null
			});
			return path;
		},

		// Get current state
		getState: () => get({ subscribe })
	};
}

export const unsavedChanges = createUnsavedChangesStore();
