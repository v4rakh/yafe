import { AccessDeniedPage } from '../pages/accessDenied';
import { useCan } from '@refinedev/core';
import { Spin } from 'antd';

interface ProtectedRouteProps {
	resource: string;
	action?: string;
	children: React.ReactNode;
}

export function ProtectedRoute({ resource, action = 'list', children }: ProtectedRouteProps) {
	const { data, isLoading } = useCan({
		resource,
		action
	});

	if (isLoading) {
		return (
			<div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%' }}>
				<Spin size="large" />
			</div>
		);
	}

	if (!data?.can) {
		return <AccessDeniedPage />;
	}

	return <>{children}</>;
}
