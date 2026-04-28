import { useGo } from '@refinedev/core';
import { Button, Result } from 'antd';
import { useTranslation } from 'react-i18next';

export function AccessDeniedPage() {
	const { t } = useTranslation();
	const go = useGo();

	return (
		<Result
			status="403"
			title={t('auth.accessDenied')}
			subTitle={t('auth.accessDeniedMessage')}
			extra={
				<Button type="primary" onClick={() => go({ to: '/' })}>
					{t('auth.goHome')}
				</Button>
			}
		/>
	);
}
