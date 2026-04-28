import type { components } from '../types/api';
import { Tag } from 'antd';
import { useTranslation } from 'react-i18next';

type JobStatus = components['schemas']['JobStatus'];

const statusColors: Record<JobStatus, string> = {
	pending: 'default',
	running: 'processing',
	done: 'success',
	failed: 'error'
};

interface StatusTagProps {
	status: JobStatus;
}

export function StatusTag({ status }: StatusTagProps) {
	const { t } = useTranslation();
	const color = statusColors[status] || 'default';
	const label = t(`jobs.statuses.${status}`, { defaultValue: status });
	return <Tag color={color}>{label}</Tag>;
}

// Infer status from job data when status field is not present
export function inferJobStatus(job: {
	started_at?: string | null;
	ended_at?: string | null;
	error?: string;
}): JobStatus {
	if (!job.started_at) return 'pending';
	if (!job.ended_at) return 'running';
	if (job.error) return 'failed';
	return 'done';
}
