import { RefreshButton } from "../../components/RefreshButton";
import { POLLING_INTERVAL_LIST } from "../../constants";
import { PlayCircleOutlined } from "@ant-design/icons";
import { List, useTable, DeleteButton, EditButton } from "@refinedev/antd";
import { useCan } from "@refinedev/core";
import { Table, Space, Button } from "antd";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

interface Flow {
  id: string;
  name: string;
}

export function FlowList() {
  const { t } = useTranslation();
  const [autoRefresh, setAutoRefresh] = useState(false);
  const { data: canRunJobs } = useCan({ resource: "jobs", action: "create" });
  const { tableProps, tableQuery } = useTable<Flow>({
    syncWithLocation: true,
    pagination: { mode: "off" },
    queryOptions: {
      refetchInterval: autoRefresh ? POLLING_INTERVAL_LIST : false,
    },
  });

  return (
    <List headerButtons={({ defaultButtons }) => (<>{defaultButtons}<RefreshButton onClick={() => tableQuery.refetch()} loading={tableQuery.isFetching} autoRefresh={autoRefresh} onAutoRefreshChange={setAutoRefresh} /></>)}>
      <Table {...tableProps} rowKey="id">
        <Table.Column dataIndex="name" title={t("common.name")} />
        <Table.Column
          title={t("common.actions")}
          render={(_, record: Flow) => (
            <Space>
              {canRunJobs?.can && (
                <Link to={`/flows/run/${record.name}`}>
                  <Button type="link" icon={<PlayCircleOutlined />} style={{ color: '#52c41a' }} />
                </Link>
              )}
              <EditButton hideText size="small" type="link" recordItemId={record.id} />
              <DeleteButton hideText size="small" type="link" recordItemId={record.id} onSuccess={() => tableQuery.refetch()} danger />
            </Space>
          )}
        />
      </Table>
    </List>
  );
}
