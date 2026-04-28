import { YamlEditor } from "../../components/YamlEditor";
import { Create, useForm } from "@refinedev/antd";
import { Form, Input } from "antd";
import { useTranslation } from "react-i18next";

const defaultFlowContent = `runs-on: host
name: my-flow
steps:
  - name: hello
    kind: shell
    cmd: echo "Hello, World!"
`;

export function FlowCreate() {
  const { t } = useTranslation();
  const { formProps, saveButtonProps } = useForm({
    redirect: "list",
  });

  return (
    <Create saveButtonProps={saveButtonProps} goBack={null}>
      <Form {...formProps} layout="vertical" initialValues={{ content: defaultFlowContent }}>
        <Form.Item
          label={t("common.name")}
          name="name"
          rules={[
            { required: true, message: t("flows.validation.nameRequired") },
            {
              pattern: /^[a-zA-Z0-9_-]+$/,
              message: t("flows.validation.namePattern"),
            },
          ]}
        >
          <Input placeholder="my-flow" />
        </Form.Item>

        <Form.Item
          label={t("flows.contentYaml")}
          name="content"
          rules={[{ required: true, message: t("flows.validation.contentRequired") }]}
        >
          <YamlEditor height="calc(100vh - 400px)" />
        </Form.Item>
      </Form>
    </Create>
  );
}
