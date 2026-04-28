import { KeyValueInputList } from "../../components/KeyValueInputList";
import { Create } from "@refinedev/antd";
import { useCreate, useNavigation } from "@refinedev/core";
import { Form, App, Typography } from "antd";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";

const { Text } = Typography;

interface FormValues {
  inputsList?: Array<{ key: string; value: string }>;
}

export function FlowRun() {
  const { t } = useTranslation();
  const { message } = App.useApp();
  const { id: flowName } = useParams<{ id: string }>();
  const [form] = Form.useForm();
  const [isLoading, setIsLoading] = useState(false);
  const { mutate: createJob } = useCreate();
  const { show } = useNavigation();

  const handleFinish = async (values: unknown) => {
    const formValues = values as FormValues;
    const inputs: Record<string, string> = {};
    if (formValues.inputsList) {
      formValues.inputsList.forEach(({ key, value }) => {
        if (key && value) {
          inputs[key] = value;
        }
      });
    }

    setIsLoading(true);
    createJob(
      {
        resource: "jobs",
        values: {
          flow: flowName,
          inputs: Object.keys(inputs).length > 0 ? inputs : undefined,
        },
        successNotification: false,
        errorNotification: false,
      },
      {
        onSuccess: (data) => {
          const jobId = data.data.id;
          message.success(t("jobs.enqueued", { id: jobId }));
          setIsLoading(false);
          if (jobId) {
            show("jobs", jobId);
          }
        },
        onError: (error) => {
          message.error(t("jobs.failedToEnqueue", { error: error.message }));
          setIsLoading(false);
        },
      }
    );
  };

  return (
    <Create
      title={t("flows.runFlow", { name: flowName })}
      saveButtonProps={{ loading: isLoading, children: t("flows.run"), onClick: () => form.submit() }}
      goBack={null}
      headerButtons={false}
    >
      <div style={{ marginBottom: 16 }}>
        <Text strong>{t("jobs.flow")}: </Text>
        <Text>{flowName}</Text>
      </div>

      <Form form={form} layout="vertical" onFinish={handleFinish}>
        <KeyValueInputList name="inputsList" />
      </Form>
    </Create>
  );
}
