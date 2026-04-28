import { KeyValueInputList } from "../../components/KeyValueInputList";
import type { components } from "../../types/api";
import { Edit, useForm, useSelect } from "@refinedev/antd";
import { Form, Input, Select, Switch, Typography } from "antd";
import { useEffect } from "react";
import { useTranslation } from "react-i18next";

type ScheduleType = components["schemas"]["ScheduleType"];
type ScheduleResponse = components["schemas"]["ScheduleResponse"];

const { Text } = Typography;

interface FormValues {
  flow: string;
  type: ScheduleType;
  expression: string;
  enabled: boolean;
  inputsList?: Array<{ key: string; value: string }>;
}

export function ScheduleEdit() {
  const { t } = useTranslation();
  const { formProps, saveButtonProps, onFinish, query, form } = useForm<ScheduleResponse>({
    redirect: "list",
  });

  const { selectProps: flowSelectProps } = useSelect({
    resource: "flows",
    optionLabel: "name",
    optionValue: "name",
  });

  // Convert inputs object to list format for editing
  useEffect(() => {
    if (query?.data?.data?.inputs) {
      const inputsList = Object.entries(query.data.data.inputs).map(([key, value]) => ({
        key,
        value,
      }));
      form.setFieldsValue({ inputsList });
    }
  }, [query?.data?.data?.inputs, form]);

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
    return onFinish({
      flow: formValues.flow,
      type: formValues.type,
      expression: formValues.expression,
      enabled: formValues.enabled,
      inputs: Object.keys(inputs).length > 0 ? inputs : undefined,
    });
  };

  return (
    <Edit saveButtonProps={saveButtonProps} goBack={null} headerButtons={() => null}>
      <div style={{ marginBottom: 16 }}>
        <Text strong>{t("common.name")}: </Text>
        <Text>{query?.data?.data?.name}</Text>
      </div>

      <Form {...formProps} layout="vertical" onFinish={handleFinish}>
        <Form.Item
          label={t("jobs.flow")}
          name="flow"
          rules={[{ required: true, message: t("schedules.validation.flowRequired") }]}
        >
          <Select {...flowSelectProps} placeholder={t("form.selectFlow")} />
        </Form.Item>

        <Form.Item
          label={t("common.type")}
          name="type"
          rules={[{ required: true, message: t("schedules.validation.typeRequired") }]}
        >
          <Select
            options={[
              { label: t("schedules.cron"), value: "cron" },
              { label: t("schedules.interval"), value: "interval" },
            ]}
          />
        </Form.Item>

        <Form.Item
          label={t("schedules.expression")}
          name="expression"
          rules={[{ required: true, message: t("schedules.validation.expressionRequired") }]}
          extra={t("schedules.expressionHint")}
        >
          <Input placeholder="0 0 * * *" />
        </Form.Item>

        <Form.Item label={t("common.enabled")} name="enabled" valuePropName="checked">
          <Switch />
        </Form.Item>

        <KeyValueInputList name="inputsList" />
      </Form>
    </Edit>
  );
}
