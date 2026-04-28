import { FlowLink } from "../../components/FlowLink";
import { RefreshButton } from "../../components/RefreshButton";
import { enableSchedule, disableSchedule } from "../../providers/dataProvider";
import type { components } from "../../types/api";
import { Show, EditButton, DeleteButton } from "@refinedev/antd";
import { useShow, useCan, useNavigation } from "@refinedev/core";
import { Typography, Descriptions, Tag, Switch, App, Space } from "antd";
import { useState } from "react";
import { useTranslation } from "react-i18next";

type ScheduleResponse = components["schemas"]["ScheduleResponse"];

const { Title } = Typography;

export function ScheduleShow() {
  const { t } = useTranslation();
  const { list } = useNavigation();
  const { message } = App.useApp();
  const [autoRefresh, setAutoRefresh] = useState(true);
  const { data: canWrite } = useCan({ resource: "schedules", action: "edit" });
  const { query } = useShow<ScheduleResponse>({
    queryOptions: {
      refetchInterval: autoRefresh ? 10000 : false,
    },
  });
  const { data, isLoading, refetch, isFetching } = query;
  const record = data?.data;
  const [toggling, setToggling] = useState(false);

  const handleToggle = async () => {
    if (!record) return;
    setToggling(true);
    try {
      if (record.enabled) {
        await disableSchedule(record.name);
        message.success(t("schedules.disabled", { name: record.name }));
      } else {
        await enableSchedule(record.name);
        message.success(t("schedules.enabled", { name: record.name }));
      }
      refetch();
    } catch (error) {
      message.error(t("schedules.failedToToggle", { error: String(error) }));
    } finally {
      setToggling(false);
    }
  };

  return (
    <Show
      isLoading={isLoading}
      goBack={null}
      headerButtons={() => (
        <Space>
          <RefreshButton onClick={() => refetch()} loading={isFetching} autoRefresh={autoRefresh} onAutoRefreshChange={setAutoRefresh} />
          <EditButton />
          <DeleteButton onSuccess={() => list("schedules")} />
        </Space>
      )}
    >
      <Title level={5}>{t("schedules.details")}</Title>
      {record && (
        <Descriptions bordered column={1}>
          <Descriptions.Item label={t("common.name")}>{record.name}</Descriptions.Item>
          <Descriptions.Item label={t("jobs.flow")}>
            <FlowLink flowName={record.flow} />
          </Descriptions.Item>
          <Descriptions.Item label={t("common.type")}>
            <Tag>{record.type}</Tag>
          </Descriptions.Item>
          <Descriptions.Item label={t("schedules.expression")}>{record.expression}</Descriptions.Item>
          <Descriptions.Item label={t("common.enabled")}>
            <Switch checked={record.enabled} loading={toggling} disabled={!canWrite?.can} onChange={handleToggle} />
          </Descriptions.Item>
          <Descriptions.Item label={t("common.created")}>
            {new Date(record.created_at).toLocaleString()}
          </Descriptions.Item>
          <Descriptions.Item label={t("schedules.nextRun")}>
            {record.next_run_at ? new Date(record.next_run_at).toLocaleString() : t("common.noData")}
          </Descriptions.Item>
          <Descriptions.Item label={t("schedules.lastRun")}>
            {record.last_run_at ? new Date(record.last_run_at).toLocaleString() : t("common.noData")}
          </Descriptions.Item>
          {record.inputs && Object.keys(record.inputs).length > 0 && (
            <Descriptions.Item label={t("common.inputs")}>
              <pre style={{ margin: 0, whiteSpace: "pre-wrap" }}>
                {JSON.stringify(record.inputs, null, 2)}
              </pre>
            </Descriptions.Item>
          )}
        </Descriptions>
      )}
    </Show>
  );
}
