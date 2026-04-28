import { RefreshButton } from "../../components/RefreshButton";
import type { components } from "../../types/api";
import { PlayCircleOutlined } from "@ant-design/icons";
import { Show, EditButton, DeleteButton } from "@refinedev/antd";
import { useShow, useCan, useNavigation } from "@refinedev/core";
import { Typography, Space, Button } from "antd";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

type FlowResponse = components["schemas"]["FlowResponse"];

const { Title, Text } = Typography;

export function FlowShow() {
  const { t } = useTranslation();
  const { list } = useNavigation();
  const [autoRefresh, setAutoRefresh] = useState(true);
  const { data: canRunJobs } = useCan({ resource: "jobs", action: "create" });
  const { query } = useShow<FlowResponse>({
    queryOptions: {
      refetchInterval: autoRefresh ? 10000 : false,
    },
  });
  const { data, isLoading } = query;
  const record = data?.data;

  return (
    <Show
      isLoading={isLoading}
      goBack={null}
      headerButtons={() => (
        <Space>
          <RefreshButton onClick={() => query.refetch()} loading={query.isFetching} autoRefresh={autoRefresh} onAutoRefreshChange={setAutoRefresh} />
          {canRunJobs?.can && (
            <Link to={`/flows/run/${record?.name}`}>
              <Button type="primary" icon={<PlayCircleOutlined />}>
                {t("flows.run")}
              </Button>
            </Link>
          )}
          <EditButton />
          <DeleteButton onSuccess={() => list("flows")} />
        </Space>
      )}
    >
      {record && (
        <>
          <Title level={5}>{t("common.name")}</Title>
          <Text>{record.name}</Text>

          <Title level={5} style={{ marginTop: 16 }}>
            {t("common.content")}
          </Title>
          <pre
            style={{
              backgroundColor: "#f5f5f5",
              padding: 16,
              borderRadius: 4,
              overflow: "auto",
              whiteSpace: "pre-wrap",
            }}
          >
            {record.content}
          </pre>
        </>
      )}
    </Show>
  );
}
