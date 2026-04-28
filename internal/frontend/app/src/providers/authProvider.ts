import type { AuthProvider } from '@refinedev/core';
import type { components } from '../types/api';
import { clearPermissionsCache } from './accessControlProvider';

type ProfileResponse = components['schemas']['ProfileResponse'];

const API_KEY_STORAGE_KEY = 'yafe_api_key';
const API_URL = '/api/v1';
const CACHE_DURATION = 300000; // 5 minutes

// Profile cache
let profileCache: {
	data: ProfileResponse | null;
	timestamp: number;
	apiKey: string | null;
} | null = null;

export function getApiKey(): string | null {
	return sessionStorage.getItem(API_KEY_STORAGE_KEY);
}

export function clearProfileCache(): void {
	profileCache = null;
}

export async function fetchProfile(apiKey?: string): Promise<ProfileResponse | null> {
	const currentApiKey = apiKey || null;

	// Check if cache is valid
	if (profileCache && profileCache.apiKey === currentApiKey && Date.now() - profileCache.timestamp < CACHE_DURATION) {
		return profileCache.data;
	}

	const headers: HeadersInit = {
		'Content-Type': 'application/json'
	};

	if (apiKey) {
		headers['X-Api-Key'] = apiKey;
	}

	const response = await fetch(`${API_URL}/profile`, { headers });

	if (response.status === 401) {
		profileCache = {
			data: null,
			timestamp: Date.now(),
			apiKey: currentApiKey
		};
		return null;
	}

	if (!response.ok) {
		throw new Error('Failed to fetch profile');
	}

	const data = await response.json();

	// Cache the result
	profileCache = {
		data,
		timestamp: Date.now(),
		apiKey: currentApiKey
	};

	return data;
}

export const authProvider: AuthProvider = {
	login: async ({ apiKey }: { apiKey: string }) => {
		clearProfileCache();
		const profile = await fetchProfile(apiKey);

		if (!profile) {
			return {
				success: false,
				error: {
					name: 'LoginError',
					message: 'Invalid API key'
				}
			};
		}

		// Empty username means auth is not required - this shouldn't happen during login
		if (!profile.username) {
			return {
				success: false,
				error: {
					name: 'LoginError',
					message: 'Authentication not required'
				}
			};
		}

		sessionStorage.setItem(API_KEY_STORAGE_KEY, apiKey);
		clearPermissionsCache();

		return {
			success: true,
			redirectTo: '/'
		};
	},

	logout: async () => {
		sessionStorage.removeItem(API_KEY_STORAGE_KEY);
		clearPermissionsCache();
		clearProfileCache();
		return {
			success: true,
			redirectTo: '/login'
		};
	},

	check: async () => {
		const apiKey = getApiKey();

		try {
			const profile = await fetchProfile(apiKey || undefined);

			if (!profile) {
				// 401 - auth required but no/invalid key
				return {
					authenticated: false,
					redirectTo: '/login'
				};
			}

			// Empty username means no auth required - consider authenticated
			if (!profile.username) {
				return {
					authenticated: true
				};
			}

			// Has username - authenticated
			return {
				authenticated: true
			};
		} catch {
			return {
				authenticated: false,
				redirectTo: '/login',
				error: {
					name: 'AuthError',
					message: 'Failed to verify authentication'
				}
			};
		}
	},

	getIdentity: async () => {
		const apiKey = getApiKey();

		try {
			const profile = await fetchProfile(apiKey || undefined);

			if (!profile || !profile.username) {
				return null;
			}

			return {
				id: profile.username,
				name: profile.username,
				roles: profile.roles
			};
		} catch {
			return null;
		}
	},

	getPermissions: async () => {
		const apiKey = getApiKey();

		try {
			const profile = await fetchProfile(apiKey || undefined);

			if (!profile) {
				return null;
			}

			return profile.roles;
		} catch {
			return null;
		}
	},

	onError: async (error) => {
		if (error?.statusCode === 401) {
			return {
				logout: true,
				redirectTo: '/login'
			};
		}

		return { error };
	}
};
