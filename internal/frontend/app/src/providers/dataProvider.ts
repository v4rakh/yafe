import type { DataProvider } from '@refinedev/core';
import type { components } from '../types/api';
import { getApiKey } from './authProvider';

type Job = components['schemas']['Job'];
type JobWithStatus = components['schemas']['JobWithStatus'];
type FlowResponse = components['schemas']['FlowResponse'];
type ScheduleResponse = components['schemas']['ScheduleResponse'];

const API_URL = '/api/v1';

async function fetcher(url: string, options?: RequestInit) {
	const apiKey = getApiKey();
	const headers: HeadersInit = {
		'Content-Type': 'application/json',
		...(apiKey && { 'X-Api-Key': apiKey }),
		...options?.headers
	};

	const response = await fetch(url, {
		...options,
		headers
	});

	if (!response.ok) {
		const error = await response.json().catch(() => ({ error: response.statusText }));
		throw new Error(error.error || 'Request failed');
	}

	// Handle responses with no body (201 Created, 204 No Content)
	if (response.status === 201 || response.status === 204) {
		return null;
	}

	// Check if response has content before parsing
	const text = await response.text();
	if (!text) {
		return null;
	}

	return JSON.parse(text);
}

export const dataProvider: DataProvider = {
	getList: async ({ resource, filters }) => {
		let url = `${API_URL}/${resource}`;

		if (resource === 'jobs' && filters) {
			const statusFilters = filters.filter((f) => 'field' in f && f.field === 'status');
			if (statusFilters.length > 0) {
				const params = new URLSearchParams();
				statusFilters.forEach((f) => {
					if ('value' in f) {
						if (Array.isArray(f.value)) {
							f.value.forEach((v: string) => params.append('status', v));
						} else {
							params.append('status', f.value as string);
						}
					}
				});
				url += `?${params.toString()}`;
			}
		}

		const data = await fetcher(url);

		if (resource === 'flows') {
			// Flows API returns array of strings (names)
			const flowNames = (data as string[]) ?? [];
			return {
				data: flowNames.map((name) => ({ id: name, name })),
				total: flowNames.length
			};
		}

		if (resource === 'jobs') {
			// Jobs don't have status in list response, but we can infer from timestamps
			const jobs = (data as Job[]) ?? [];
			return {
				data: jobs.map((job) => ({ ...job, id: job.id })),
				total: jobs.length
			};
		}

		if (resource === 'schedules') {
			const schedules = (data as ScheduleResponse[]) ?? [];
			return {
				data: schedules.map((s) => ({ ...s, id: s.name })),
				total: schedules.length
			};
		}

		return {
			data: data,
			total: Array.isArray(data) ? data.length : 0
		};
	},

	getOne: async ({ resource, id }) => {
		if (resource === 'flows') {
			const data = (await fetcher(`${API_URL}/flows/${id}`)) as FlowResponse;
			return { data: { ...data, id: data.name } };
		}

		if (resource === 'jobs') {
			const data = (await fetcher(`${API_URL}/jobs/${id}`)) as JobWithStatus;
			return { data: { ...data, id: data.id } };
		}

		if (resource === 'schedules') {
			const data = (await fetcher(`${API_URL}/schedules/${id}`)) as ScheduleResponse;
			return { data: { ...data, id: data.name } };
		}

		const data = await fetcher(`${API_URL}/${resource}/${id}`);
		return { data };
	},

	create: async ({ resource, variables }) => {
		if (resource === 'jobs') {
			// POST /jobs returns { job_id }
			const result = await fetcher(`${API_URL}/jobs`, {
				method: 'POST',
				body: JSON.stringify(variables)
			});
			return { data: { id: result.job_id } };
		}

		if (resource === 'flows') {
			// PUT /flows/{name} for create/update
			const { name, content } = variables as { name: string; content: string };
			await fetcher(`${API_URL}/flows/${name}`, {
				method: 'PUT',
				body: JSON.stringify({ content })
			});
			return { data: { id: name, name, content } };
		}

		if (resource === 'schedules') {
			await fetcher(`${API_URL}/schedules`, {
				method: 'POST',
				body: JSON.stringify(variables)
			});
			const { name } = variables as { name: string };
			return { data: { id: name, ...(variables as object) } };
		}

		const data = await fetcher(`${API_URL}/${resource}`, {
			method: 'POST',
			body: JSON.stringify(variables)
		});
		return { data };
	},

	update: async ({ resource, id, variables }) => {
		if (resource === 'flows') {
			const { content } = variables as { content: string };
			await fetcher(`${API_URL}/flows/${id}`, {
				method: 'PUT',
				body: JSON.stringify({ content })
			});
			return { data: { id, name: id, content } };
		}

		if (resource === 'schedules') {
			await fetcher(`${API_URL}/schedules/${id}`, {
				method: 'PUT',
				body: JSON.stringify(variables)
			});
			return { data: { id, ...(variables as object) } };
		}

		const data = await fetcher(`${API_URL}/${resource}/${id}`, {
			method: 'PUT',
			body: JSON.stringify(variables)
		});
		return { data };
	},

	deleteOne: async ({ resource, id }) => {
		await fetcher(`${API_URL}/${resource}/${id}`, { method: 'DELETE' });
		// Type assertion needed due to Refine's generic TData constraint
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		return { data: { id } } as any;
	},

	getApiUrl: () => API_URL,

	// Custom method for schedule enable/disable
	custom: async ({ url, method, payload }) => {
		const data = await fetcher(url, {
			method,
			body: payload ? JSON.stringify(payload) : undefined
		});
		return { data };
	}
};

// Helper functions for schedule enable/disable
export async function enableSchedule(name: string): Promise<void> {
	await fetcher(`${API_URL}/schedules/${name}/enable`, { method: 'POST' });
}

export async function disableSchedule(name: string): Promise<void> {
	await fetcher(`${API_URL}/schedules/${name}/disable`, { method: 'POST' });
}

// Helper function for flow rename
export async function renameFlow(oldName: string, newName: string): Promise<void> {
	await fetcher(`${API_URL}/flows/${oldName}/rename`, {
		method: 'POST',
		body: JSON.stringify({ new_name: newName })
	});
}
