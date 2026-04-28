import type { AccessControlProvider } from '@refinedev/core';
import { getApiKey, fetchProfile } from './authProvider';

const resourceRoleMap: Record<string, string> = {
	jobs: 'jobs:read',
	flows: 'flows:read',
	schedules: 'schedules:read'
};

// Cache for permissions to avoid fetching on every check
let cachedPermissions: string[] | null = null;
let cacheKey: string | null = null;

async function getPermissions(): Promise<string[]> {
	const apiKey = getApiKey();
	const currentCacheKey = apiKey || 'no-key';

	// Return cached if same key
	if (cacheKey === currentCacheKey && cachedPermissions !== null) {
		return cachedPermissions;
	}

	try {
		const profile = await fetchProfile(apiKey || undefined);
		cachedPermissions = profile?.roles || [];
		cacheKey = currentCacheKey;
		return cachedPermissions;
	} catch {
		cachedPermissions = [];
		cacheKey = currentCacheKey;
		return [];
	}
}

// Clear cache when user logs in/out
export function clearPermissionsCache() {
	cachedPermissions = null;
	cacheKey = null;
}

export const accessControlProvider: AccessControlProvider = {
	can: async ({ resource, action }) => {
		const permissions = await getPermissions();

		// If no permissions (empty array), check if auth is required
		if (permissions.length === 0) {
			const apiKey = getApiKey();
			if (!apiKey) {
				// No API key - fetch profile to check if auth is required
				// Use cached fetchProfile instead of direct fetch
				try {
					const profile = await fetchProfile();
					// Empty username means no auth required
					if (profile && !profile.username) {
						return { can: true };
					}
				} catch {
					// If we can't fetch, deny access
				}
			}
			// User has no roles - deny access to protected resources
			if (resource && resourceRoleMap[resource]) {
				return {
					can: false,
					reason: `Missing permission: ${resourceRoleMap[resource]}`
				};
			}
			return { can: true };
		}

		// Check if resource requires a specific role
		if (resource && resourceRoleMap[resource]) {
			const requiredRole = resourceRoleMap[resource];

			// For write actions, check write permission
			let roleToCheck = requiredRole;
			if (action === 'create' || action === 'edit' || action === 'delete') {
				roleToCheck = requiredRole.replace(':read', ':write');
			}

			if (permissions.includes(roleToCheck)) {
				return { can: true };
			}

			return {
				can: false,
				reason: roleToCheck
			};
		}

		return { can: true };
	},

	options: {
		buttons: {
			enableAccessControl: true,
			hideIfUnauthorized: true
		}
	}
};
