import { FlowLink } from "../../components/FlowLink";
import { RefreshButton } from "../../components/RefreshButton";
import { StatusTag, inferJobStatus } from "../../components/StatusTag";
import { POLLING_INTERVAL_LIST } from "../../constants";
import type { components } from "../../types/api";
import { List, useTable, DeleteButton, ShowButton } from "@refinedev/antd";
import { Table, Space } from "antd";
import { useState } from "react";
import { useTranslation } from "react-i18next";

type Job = components["schemas"]["Job"];

export function JobList() {
  const { t } = useTranslation();
  const [autoRefresh, setAutoRefresh] = useState(false);
  const { tableProps, tableQuery } = useTable<Job>({
    syncWithLocation: true,
    pagination: {
      mode: "client",
      pageSize: 15,
    },
    queryOptions: {
      refetchInterval: autoRefresh ? POLLING_INTERVAL_LIST : false,
    },
  });

  return (
    <List headerButtons={({ defaultButtons }) => (<>{defaultButtons}<RefreshButton onClick={() => tableQuery.refetch()} loading={tableQuery.isFetching} autoRefresh={autoRefresh} onAutoRefreshChange={setAutoRefresh} /></>)}>
      <Table
        {...tableProps}
        rowKey="id"
        pagination={{
          ...tableProps.pagination,
          pageSizeOptions: [15, 30, 50, 100, 250],
          showSizeChanger: true,
          showTotal: (total) => t("common.totalItems", { total }),
        }}
      >
        <Table.Column dataIndex="id" title={t("jobs.id")} />
        <Table.Column
          dataIndex="flow"
          title={t("jobs.flow")}
          render={(flowName: string) => <FlowLink flowName={flowName} />}
        />
        <Table.Column
          title={t("jobs.status")}
          render={(_, record: Job) => <StatusTag status={inferJobStatus(record)} />}
        />
        <Table.Column
          dataIndex="created_at"
          title={t("common.created")}
          render={(value: string) => new Date(value).toLocaleString()}
        />
        <Table.Column
          dataIndex="started_at"
          title={t("jobs.started")}
          render={(value: string | null) => (value ? new Date(value).toLocaleString() : t("common.noData"))}
        />
        <Table.Column
          dataIndex="ended_at"
          title={t("jobs.ended")}
          render={(value: string | null) => (value ? new Date(value).toLocaleString() : t("common.noData"))}
        />
        <Table.Column
          title={t("common.actions")}
          render={(_, record: Job) => (
            <Space>
              <ShowButton hideText size="small" type="link" recordItemId={record.id} />
              <DeleteButton hideText size="small" type="link" recordItemId={record.id} onSuccess={() => tableQuery.refetch()} danger />
            </Space>
          )}
        />
      </Table>
    </List>
  );
}
