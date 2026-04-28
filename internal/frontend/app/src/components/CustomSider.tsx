import { useMenu, useCan } from '@refinedev/core';
import { Menu, Layout, theme, MenuProps } from 'antd';
import { Link, useLocation } from 'react-router-dom';

const { Sider } = Layout;

const styles = {
	logo: {
		padding: '12px 24px',
		fontWeight: 'bold' as const,
		fontSize: 18
	},
	version: {
		position: 'absolute' as const,
		bottom: 0,
		width: '100%',
		padding: '12px 24px',
		fontSize: 12,
		color: '#8c8c8c',
		textAlign: 'center' as const
	}
} as const;

interface MenuItemWithAccess {
	key: string;
	label: React.ReactNode;
	icon?: React.ReactNode;
	resource: string;
	route: string;
	canAccess: boolean;
}

function useMenuItemsWithAccess() {
	const { menuItems } = useMenu();

	// Check access for each resource
	const jobsCan = useCan({ resource: 'jobs', action: 'list' });
	const flowsCan = useCan({ resource: 'flows', action: 'list' });
	const schedulesCan = useCan({ resource: 'schedules', action: 'list' });

	const accessMap: Record<string, boolean> = {
		jobs: jobsCan.data?.can ?? true,
		flows: flowsCan.data?.can ?? true,
		schedules: schedulesCan.data?.can ?? true
	};

	const items: MenuItemWithAccess[] = menuItems.map((item) => ({
		key: item.key ?? item.name,
		label: item.label,
		icon: item.icon,
		resource: item.name,
		route: item.route ?? `/${item.name}`,
		canAccess: accessMap[item.name] ?? true
	}));

	return { items };
}

export function CustomSider() {
	const { token } = theme.useToken();
	const location = useLocation();
	const { items } = useMenuItemsWithAccess();

	// Filter items based on access
	const visibleItems = items.filter((item) => item.canAccess);

	const menuItems: MenuProps['items'] = visibleItems.map((item) => ({
		key: item.key,
		icon: item.icon,
		label: <Link to={item.route}>{item.label}</Link>
	}));

	// Determine selected key from current path
	const selectedKey = items.find((item) => location.pathname.startsWith(item.route))?.key;

	return (
		<Sider
			style={{
				backgroundColor: token.colorBgContainer,
				borderRight: `1px solid ${token.colorBorderSecondary}`,
				position: 'relative'
			}}>
			<div
				style={{
					...styles.logo,
					borderBottom: `1px solid ${token.colorBorderSecondary}`
				}}>
				YaFE
			</div>
			<Menu
				mode="inline"
				selectedKeys={selectedKey ? [selectedKey] : []}
				items={menuItems}
				style={{ borderRight: 0 }}
			/>
			<div
				style={{
					...styles.version,
					borderTop: `1px solid ${token.colorBorderSecondary}`
				}}>
				v{__APP_VERSION__}
			</div>
		</Sider>
	);
}
