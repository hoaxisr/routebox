import { writable, get } from 'svelte/store';
import { api } from '$lib/api/client';

export interface UnsavedChange {
	section: string;      // e.g., 'endpoints', 'outbounds', 'routes'
	description: string;  // e.g., 'Added endpoint "MyVPN"'
}

interface UnsavedChangesState {
	hasChanges: boolean;
	changeCount: number;
	changes: UnsavedChange[];      // Local descriptions (for UI hints)
	loading: boolean;
}

function createUnsavedChangesStore() {
	const { subscribe, set, update } = writable<UnsavedChangesState>({
		hasChanges: false,
		changeCount: 0,
		changes: [],
		loading: false
	});

	return {
		subscribe,

		// Refresh status from backend
		refresh: async () => {
			try {
				const status = await api.getConfigStatus();
				update(state => ({
					...state,
					hasChanges: status.hasDraft,
					changeCount: status.changeCount,
					// Keep local changes if backend says we have draft, otherwise clear
					changes: status.hasDraft ? state.changes : []
				}));
			} catch {
				// Ignore errors - backend might not be available
			}
		},

		// Mark that changes have been made (local tracking for UI)
		markChanged: (section: string, description: string) => {
			update(state => ({
				...state,
				hasChanges: true,
				changeCount: state.changeCount + 1,
				changes: [...state.changes.filter(c => c.description !== description), { section, description }]
			}));
		},

		// Clear all unsaved changes (after save or discard)
		clearChanges: () => {
			set({
				hasChanges: false,
				changeCount: 0,
				changes: [],
				loading: false
			});
		},

		// Set loading state
		setLoading: (loading: boolean) => {
			update(state => ({ ...state, loading }));
		},

		// Get current state
		getState: () => get({ subscribe })
	};
}

export const unsavedChanges = createUnsavedChangesStore();
