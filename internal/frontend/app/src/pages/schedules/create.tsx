import { KeyValueInputList } from "../../components/KeyValueInputList";
import type { components } from "../../types/api";
import { Create, useForm, useSelect } from "@refinedev/antd";
import { Form, Input, Select, Switch } from "antd";
import { useTranslation } from "react-i18next";

type ScheduleType = components["schemas"]["ScheduleType"];

interface FormValues {
  name: string;
  flow: string;
  type: ScheduleType;
  expression: string;
  enabled: boolean;
  inputsList?: Array<{ key: string; value: string }>;
}

export function ScheduleCreate() {
  const { t } = useTranslation();
  const { formProps, saveButtonProps, onFinish } = useForm({
    redirect: "list",
  });

  const { selectProps: flowSelectProps } = useSelect({
    resource: "flows",
    optionLabel: "name",
    optionValue: "name",
  });

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
      name: formValues.name,
      flow: formValues.flow,
      type: formValues.type,
      expression: formValues.expression,
      enabled: formValues.enabled ?? false,
      inputs: Object.keys(inputs).length > 0 ? inputs : undefined,
    });
  };

  return (
    <Create saveButtonProps={saveButtonProps} goBack={null}>
      <Form {...formProps} layout="vertical" onFinish={handleFinish} initialValues={{ enabled: false, type: "cron" }}>
        <Form.Item
          label={t("common.name")}
          name="name"
          rules={[
            { required: true, message: t("schedules.validation.nameRequired") },
            {
              pattern: /^[a-zA-Z0-9_-]+$/,
              message: t("schedules.validation.namePattern"),
            },
          ]}
        >
          <Input placeholder="daily-backup" />
        </Form.Item>

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
    </Create>
  );
}
