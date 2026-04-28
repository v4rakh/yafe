import { FlowLink } from "../../components/FlowLink";
import { RefreshButton } from "../../components/RefreshButton";
import { StatusTag } from "../../components/StatusTag";
import { POLLING_INTERVAL_DETAIL } from "../../constants";
import { getApiKey } from "../../providers/authProvider";
import type { components } from "../../types/api";
import { Show, DeleteButton } from "@refinedev/antd";
import { useShow, useNavigation } from "@refinedev/core";
import { Typography, Descriptions, Alert, Collapse, Spin, Space } from "antd";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

type JobWithStatus = components["schemas"]["JobWithStatus"];

const { Title } = Typography;

const styles = {
  pre: {
    margin: 0,
    whiteSpace: "pre-wrap" as const,
  },
  logsContainer: {
    margin: 0,
    padding: 12,
    backgroundColor: "#1e1e1e",
    color: "#d4d4d4",
    borderRadius: 4,
    overflow: "auto" as const,
    maxHeight: 500,
    whiteSpace: "pre-wrap" as const,
    wordBreak: "break-all" as const,
    fontSize: 12,
    fontFamily: "monospace",
  },
  marginTop: {
    marginTop: 16,
  },
} as const;

export function JobShow() {
  const { t } = useTranslation();
  const { list } = useNavigation();
  const [autoRefresh, setAutoRefresh] = useState(true);
  const { query } = useShow<JobWithStatus>({
    queryOptions: {
      refetchInterval: autoRefresh ? POLLING_INTERVAL_DETAIL : false,
    },
  });
  const { data, isLoading } = query;
  const record = data?.data;

  const [logs, setLogs] = useState<string | null>(null);
  const [logsLoading, setLogsLoading] = useState(false);
  const [logsError, setLogsError] = useState<string | null>(null);

  const canShowLogs = record?.status === "done" || record?.status === "failed";

  useEffect(() => {
    if (!record?.id || !canShowLogs) {
      setLogs(null);
      setLogsError(null);
      return;
    }

    const controller = new AbortController();

    const fetchLogs = async () => {
      setLogsLoading(true);
      setLogsError(null);
      try {
        const apiKey = getApiKey();
        const headers: HeadersInit = {};
        if (apiKey) {
          headers["X-Api-Key"] = apiKey;
        }
        const response = await fetch(`/api/v1/jobs/${record.id}/logs`, {
          headers,
          signal: controller.signal,
        });
        if (!response.ok) {
          if (response.status === 425) {
            setLogsError(t("jobs.logsNotYetAvailable"));
          } else if (response.status === 404) {
            setLogsError(t("jobs.logsNotFound"));
          } else {
            setLogsError(t("jobs.failedToFetchLogs", { error: response.statusText }));
          }
          return;
        }
        const text = await response.text();
        setLogs(text);
      } catch (err) {
        if (err instanceof Error && err.name === "AbortError") {
          return;
        }
        setLogsError(t("jobs.failedToFetchLogs", { error: String(err) }));
      } finally {
        setLogsLoading(false);
      }
    };

    fetchLogs();

    return () => controller.abort();
  }, [record?.id, canShowLogs, t]);

  return (
    <Show isLoading={isLoading} goBack={null} headerButtons={() => (<Space><RefreshButton onClick={() => query.refetch()} loading={query.isFetching} autoRefresh={autoRefresh} onAutoRefreshChange={setAutoRefresh} /><DeleteButton onSuccess={() => list("jobs")} /></Space>)}>
      <Title level={5}>{t("jobs.details")}</Title>
      {record && (
        <>
          <Descriptions bordered column={1}>
            <Descriptions.Item label={t("jobs.id")}>{record.id}</Descriptions.Item>
            <Descriptions.Item label={t("jobs.flow")}>
              <FlowLink flowName={record.flow} />
            </Descriptions.Item>
            <Descriptions.Item label={t("jobs.status")}>
              <StatusTag status={record.status} />
            </Descriptions.Item>
            <Descriptions.Item label={t("common.created")}>
              {new Date(record.created_at).toLocaleString()}
            </Descriptions.Item>
            <Descriptions.Item label={t("jobs.started")}>
              {record.started_at ? new Date(record.started_at).toLocaleString() : t("common.noData")}
            </Descriptions.Item>
            <Descriptions.Item label={t("jobs.ended")}>
              {record.ended_at ? new Date(record.ended_at).toLocaleString() : t("common.noData")}
            </Descriptions.Item>
            <Descriptions.Item label={t("jobs.exitCode")}>
              {record.exit_code !== null && record.exit_code !== undefined
                ? record.exit_code
                : t("common.noData")}
            </Descriptions.Item>
            {record.inputs && Object.keys(record.inputs).length > 0 && (
              <Descriptions.Item label={t("common.inputs")}>
                <pre style={styles.pre}>
                  {JSON.stringify(record.inputs, null, 2)}
                </pre>
              </Descriptions.Item>
            )}
          </Descriptions>

          {record.error && (
            <Alert
              style={styles.marginTop}
              type="error"
              message={t("common.error")}
              description={<pre style={styles.pre}>{record.error}</pre>}
            />
          )}

          {canShowLogs && (
            <Collapse
              style={styles.marginTop}
              defaultActiveKey={["logs"]}
              items={[
                {
                  key: "logs",
                  label: t("jobs.logs"),
                  children: logsLoading ? (
                    <Spin />
                  ) : logsError ? (
                    <Alert type="warning" message={logsError} />
                  ) : logs ? (
                    <pre style={styles.logsContainer}>
                      {logs}
                    </pre>
                  ) : (
                    <Alert type="info" message={t("jobs.noLogsAvailable")} />
                  ),
                },
              ]}
            />
          )}

          {!canShowLogs && (record.status === "pending" || record.status === "running") && (
            <Alert
              style={styles.marginTop}
              type="info"
              message={t("jobs.logsAvailableAfterCompletion")}
            />
          )}
        </>
      )}
    </Show>
  );
}
