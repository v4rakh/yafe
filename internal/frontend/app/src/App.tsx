import { CustomSider } from './components/CustomSider';
import { ProtectedRoute } from './components/ProtectedRoute';
import { UserHeader } from './components/UserHeader';
import { FlowList, FlowCreate, FlowEdit, FlowRun } from './pages/flows';
import { JobList, JobShow } from './pages/jobs';
import { LoginPage } from './pages/login';
import { ScheduleList, ScheduleShow, ScheduleCreate, ScheduleEdit } from './pages/schedules';
import { accessControlProvider } from './providers/accessControlProvider';
import { authProvider } from './providers/authProvider';
import { dataProvider } from './providers/dataProvider';
import { useAppNotificationProvider } from './providers/notificationProvider';
import { CalendarOutlined, ThunderboltOutlined, ApartmentOutlined } from '@ant-design/icons';
import { ThemedLayout, RefineThemes, ErrorComponent } from '@refinedev/antd';
import { Refine, Authenticated } from '@refinedev/core';
import routerProvider, {
	NavigateToResource,
	UnsavedChangesNotifier,
	DocumentTitleHandler,
	CatchAllNavigate
} from '@refinedev/react-router';
import { App as AntdApp, ConfigProvider } from 'antd';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { BrowserRouter, Routes, Route, Outlet } from 'react-router-dom';
import '@refinedev/antd/dist/reset.css';
import './i18n';

const titleSuffix = ' | YaFE';

const styles = {
	header: {
		display: 'flex',
		alignItems: 'center',
		justifyContent: 'flex-end',
		padding: '0 24px',
		height: 64,
		background: '#fff',
		borderBottom: '1px solid #f0f0f0'
	}
} as const;

function HeaderContent() {
	return (
		<div style={styles.header}>
			<UserHeader />
		</div>
	);
}

function RefineApp() {
	const notificationProvider = useAppNotificationProvider();
	const { t, i18n } = useTranslation();

	const i18nProvider = useMemo(
		() => ({
			translate: (key: string, options?: Record<string, unknown>) => t(key, options as Record<string, string>),
			changeLocale: (lang: string) => i18n.changeLanguage(lang),
			getLocale: () => i18n.language
		}),
		[t, i18n]
	);

	return (
		<Refine
			routerProvider={routerProvider}
			dataProvider={dataProvider}
			authProvider={authProvider}
			accessControlProvider={accessControlProvider}
			notificationProvider={notificationProvider}
			i18nProvider={i18nProvider}
			resources={[
				{
					name: 'flows',
					list: '/flows',
					create: '/flows/create',
					edit: '/flows/edit/:id',
					meta: {
						icon: <ApartmentOutlined />,
						label: t('flows.flows')
					}
				},
				{
					name: 'jobs',
					list: '/jobs',
					show: '/jobs/show/:id',
					meta: {
						icon: <ThunderboltOutlined />,
						label: t('jobs.jobs')
					}
				},
				{
					name: 'schedules',
					list: '/schedules',
					show: '/schedules/show/:id',
					create: '/schedules/create',
					edit: '/schedules/edit/:id',
					meta: {
						icon: <CalendarOutlined />,
						label: t('schedules.schedules')
					}
				}
			]}
			options={{
				syncWithLocation: true,
				warnWhenUnsavedChanges: true,
				disableTelemetry: true
			}}>
			<Routes>
				<Route
					element={
						<Authenticated key="authenticated-routes" fallback={<CatchAllNavigate to="/login" />}>
							<ThemedLayout Sider={CustomSider} Header={HeaderContent}>
								<Outlet />
							</ThemedLayout>
						</Authenticated>
					}>
					<Route index element={<NavigateToResource resource="flows" />} />

					<Route path="/flows">
						<Route
							index
							element={
								<ProtectedRoute resource="flows">
									<FlowList />
								</ProtectedRoute>
							}
						/>
						<Route
							path="create"
							element={
								<ProtectedRoute resource="flows" action="create">
									<FlowCreate />
								</ProtectedRoute>
							}
						/>
						<Route
							path="edit/:id"
							element={
								<ProtectedRoute resource="flows" action="edit">
									<FlowEdit />
								</ProtectedRoute>
							}
						/>
						<Route
							path="run/:id"
							element={
								<ProtectedRoute resource="jobs" action="create">
									<FlowRun />
								</ProtectedRoute>
							}
						/>
					</Route>

					<Route path="/jobs">
						<Route
							index
							element={
								<ProtectedRoute resource="jobs">
									<JobList />
								</ProtectedRoute>
							}
						/>
						<Route
							path="show/:id"
							element={
								<ProtectedRoute resource="jobs" action="show">
									<JobShow />
								</ProtectedRoute>
							}
						/>
					</Route>

					<Route path="/schedules">
						<Route
							index
							element={
								<ProtectedRoute resource="schedules">
									<ScheduleList />
								</ProtectedRoute>
							}
						/>
						<Route
							path="show/:id"
							element={
								<ProtectedRoute resource="schedules" action="show">
									<ScheduleShow />
								</ProtectedRoute>
							}
						/>
						<Route
							path="create"
							element={
								<ProtectedRoute resource="schedules" action="create">
									<ScheduleCreate />
								</ProtectedRoute>
							}
						/>
						<Route
							path="edit/:id"
							element={
								<ProtectedRoute resource="schedules" action="edit">
									<ScheduleEdit />
								</ProtectedRoute>
							}
						/>
					</Route>

					<Route path="*" element={<ErrorComponent />} />
				</Route>

				<Route
					element={
						<Authenticated key="auth-pages" fallback={<Outlet />}>
							<NavigateToResource resource="jobs" />
						</Authenticated>
					}>
					<Route path="/login" element={<LoginPage />} />
				</Route>
			</Routes>
			<UnsavedChangesNotifier />
			<DocumentTitleHandler
				handler={({ resource, action }) => {
					const resourceLabel = resource?.meta?.label ?? resource?.name ?? '';
					const singularLabel = resourceLabel.replace(/s$/, '');

					switch (action) {
						case 'list':
							return `${resourceLabel} ${titleSuffix}`;
						case 'show':
							return `${singularLabel} Details ${titleSuffix}`;
						case 'edit':
							return `Edit ${singularLabel} ${titleSuffix}`;
						case 'create':
							return `Create ${singularLabel} ${titleSuffix}`;
						default:
							return 'YaFE';
					}
				}}
			/>
		</Refine>
	);
}

export default function App() {
	return (
		<BrowserRouter>
			<ConfigProvider theme={RefineThemes.Purple}>
				<AntdApp>
					<RefineApp />
				</AntdApp>
			</ConfigProvider>
		</BrowserRouter>
	);
}
