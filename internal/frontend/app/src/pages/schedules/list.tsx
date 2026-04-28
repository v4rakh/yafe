import { FlowLink } from "../../components/FlowLink";
import { RefreshButton } from "../../components/RefreshButton";
import { POLLING_INTERVAL_LIST } from "../../constants";
import { enableSchedule, disableSchedule } from "../../providers/dataProvider";
import type { components } from "../../types/api";
import { List, useTable, DeleteButton, ShowButton, EditButton } from "@refinedev/antd";
import { useCan } from "@refinedev/core";
import { Table, Space, Switch, Tag, App } from "antd";
import { useState, useCallback } from "react";
import { useTranslation } from "react-i18next";

type ScheduleResponse = components["schemas"]["ScheduleResponse"];

export function ScheduleList() {
  const { t } = useTranslation();
  const { message } = App.useApp();
  const [autoRefresh, setAutoRefresh] = useState(false);
  const { data: canWrite } = useCan({ resource: "schedules", action: "edit" });
  const { tableProps, tableQuery } = useTable<ScheduleResponse>({
    syncWithLocation: true,
    pagination: { mode: "off" },
    queryOptions: {
      refetchInterval: autoRefresh ? POLLING_INTERVAL_LIST : false,
    },
  });
  const [loadingIds, setLoadingIds] = useState<Set<string>>(new Set());

  const handleToggle = useCallback(
    async (record: ScheduleResponse) => {
      setLoadingIds((prev) => new Set(prev).add(record.name));
      try {
        if (record.enabled) {
          await disableSchedule(record.name);
          message.success(t("schedules.disabled", { name: record.name }));
        } else {
          await enableSchedule(record.name);
          message.success(t("schedules.enabled", { name: record.name }));
        }
        tableQuery.refetch();
      } catch (error) {
        message.error(t("schedules.failedToToggle", { error: String(error) }));
      } finally {
        setLoadingIds((prev) => {
          const next = new Set(prev);
          next.delete(record.name);
          return next;
        });
      }
    },
    [message, t, tableQuery]
  );

  return (
    <List headerButtons={({ defaultButtons }) => (<>{defaultButtons}<RefreshButton onClick={() => tableQuery.refetch()} loading={tableQuery.isFetching} autoRefresh={autoRefresh} onAutoRefreshChange={setAutoRefresh} /></>)}>
      <Table {...tableProps} rowKey="name">
        <Table.Column dataIndex="name" title={t("common.name")} />
        <Table.Column
          dataIndex="flow"
          title={t("jobs.flow")}
          render={(flowName: string) => <FlowLink flowName={flowName} />}
        />
        <Table.Column
          dataIndex="type"
          title={t("common.type")}
          render={(value: string) => <Tag>{value}</Tag>}
        />
        <Table.Column dataIndex="expression" title={t("schedules.expression")} />
        <Table.Column
          dataIndex="enabled"
          title={t("common.enabled")}
          render={(enabled: boolean, record: ScheduleResponse) => (
            <Switch
              checked={enabled}
              loading={loadingIds.has(record.name)}
              disabled={!canWrite?.can}
              onChange={() => handleToggle(record)}
            />
          )}
        />
        <Table.Column
          dataIndex="next_run_at"
          title={t("schedules.nextRun")}
          render={(value: string | null) => (value ? new Date(value).toLocaleString() : t("common.noData"))}
        />
        <Table.Column
          dataIndex="last_run_at"
          title={t("schedules.lastRun")}
          render={(value: string | null) => (value ? new Date(value).toLocaleString() : t("common.noData"))}
        />
        <Table.Column
          title={t("common.actions")}
          render={(_, record: ScheduleResponse) => (
            <Space>
              <ShowButton hideText size="small" type="link" recordItemId={record.name} />
              <EditButton hideText size="small" type="link" recordItemId={record.name} />
              <DeleteButton hideText size="small" type="link" recordItemId={record.name} onSuccess={() => tableQuery.refetch()} danger />
            </Space>
          )}
        />
      </Table>
    </List>
  );
}
