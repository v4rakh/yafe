import { UserOutlined, LogoutOutlined } from '@ant-design/icons';
import { useLogout, useGetIdentity } from '@refinedev/core';
import { Button, Space, Typography, Dropdown, MenuProps } from 'antd';
import { useTranslation } from 'react-i18next';

const { Text } = Typography;

interface UserIdentity {
	id: string;
	name: string;
	roles: string[];
}

export function UserHeader() {
	const { t } = useTranslation();
	const { data: identity } = useGetIdentity<UserIdentity>();
	const { mutate: logout, isPending } = useLogout();

	// Don't show anything if no user is logged in (auth not required)
	if (!identity?.name) {
		return null;
	}

	const items: MenuProps['items'] = [
		{
			key: 'logout',
			label: t('auth.logout'),
			icon: <LogoutOutlined />,
			onClick: () => logout(),
			disabled: isPending
		}
	];

	return (
		<Dropdown menu={{ items }} placement="bottomRight" trigger={['click']}>
			<Button type="text" style={{ height: 'auto', padding: '4px 12px' }}>
				<Space>
					<UserOutlined />
					<Text>{identity.name}</Text>
				</Space>
			</Button>
		</Dropdown>
	);
}
