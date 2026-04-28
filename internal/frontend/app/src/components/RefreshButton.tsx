import { ReloadOutlined } from '@ant-design/icons';
import { Button, Switch, Space } from 'antd';
import { useTranslation } from 'react-i18next';

interface RefreshButtonProps {
	onClick: () => void;
	loading?: boolean;
	autoRefresh?: boolean;
	onAutoRefreshChange?: (enabled: boolean) => void;
}

export function RefreshButton({ onClick, loading, autoRefresh = true, onAutoRefreshChange }: RefreshButtonProps) {
	const { t } = useTranslation();

	return (
		<Space>
			{onAutoRefreshChange && (
				<Switch
					checked={autoRefresh}
					onChange={onAutoRefreshChange}
					checkedChildren={t('buttons.autoRefresh')}
					unCheckedChildren={t('buttons.autoRefresh')}
				/>
			)}
			<Button type="link" icon={<ReloadOutlined />} onClick={onClick} loading={loading}>
				{t('buttons.refresh')}
			</Button>
		</Space>
	);
}
