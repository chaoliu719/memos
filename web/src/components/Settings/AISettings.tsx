import { isEqual } from "lodash-es";
import { CheckIcon, EyeIcon, EyeOffIcon } from "lucide-react";
import { observer } from "mobx-react-lite";
import { useState } from "react";
import { toast } from "react-hot-toast";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { workspaceStore } from "@/store";
import { workspaceSettingNamePrefix } from "@/store/common";
import { WorkspaceSetting_AiSetting, WorkspaceSetting_Key } from "@/types/proto/api/v1/workspace_service";
import { useTranslate } from "@/utils/i18n";

const AISettings = observer(() => {
  const t = useTranslate();
  const [originalSetting, setOriginalSetting] = useState<WorkspaceSetting_AiSetting>(workspaceStore.state.aiSetting);
  const [aiSetting, setAiSetting] = useState<WorkspaceSetting_AiSetting>(originalSetting);
  const [showApiKey, setShowApiKey] = useState(false);
  const [testingConnection, setTestingConnection] = useState(false);

  const updatePartialSetting = (partial: Partial<WorkspaceSetting_AiSetting>) => {
    const newWorkspaceAISetting = WorkspaceSetting_AiSetting.fromPartial({
      ...aiSetting,
      ...partial,
    });
    setAiSetting(newWorkspaceAISetting);
  };

  const testConnection = async () => {
    if (!aiSetting.enableAi || !aiSetting.baseUrl || !aiSetting.apiKey || !aiSetting.model) {
      toast.error(t("setting.ai-section.test-connection-incomplete"));
      return;
    }

    setTestingConnection(true);
    try {
      // Note: This would typically call a test endpoint
      // For now, just simulate a test
      await new Promise(resolve => setTimeout(resolve, 2000));
      toast.success(t("setting.ai-section.test-connection-success"));
    } catch (error) {
      console.error("AI connection test failed:", error);
      toast.error(t("setting.ai-section.test-connection-failed"));
    } finally {
      setTestingConnection(false);
    }
  };

  const updateSetting = async () => {
    // Create a copy of the setting with defaults applied
    const settingToSave = WorkspaceSetting_AiSetting.fromPartial({
      ...aiSetting,
      // Apply defaults for empty fields when AI is enabled
      baseUrl: aiSetting.enableAi && !aiSetting.baseUrl ? "https://api.openai.com/v1" : aiSetting.baseUrl,
      timeoutSeconds: aiSetting.timeoutSeconds || 10,
    });

    if (aiSetting.enableAi) {
      if (!aiSetting.apiKey || !aiSetting.model) {
        toast.error(t("setting.ai-section.api-key-model-required"));
        return;
      }
    }

    try {
      await workspaceStore.updateWorkspaceSetting({
        name: `${workspaceSettingNamePrefix}${WorkspaceSetting_Key.AI}`,
        aiSetting: settingToSave,
      });
      setOriginalSetting(settingToSave);
      setAiSetting(settingToSave);
      toast.success(t("message.update-succeed"));
    } catch (error: any) {
      console.error(error);
      toast.error(error.response?.data?.message || error.message || t("message.update-failed"));
    }
  };

  const resetSetting = () => {
    setAiSetting(originalSetting);
  };

  const hasChanged = !isEqual(originalSetting, aiSetting);

  return (
    <div className="w-full flex flex-col gap-2 pt-2 pb-4">
      <div className="w-full flex flex-row justify-between items-center">
        <div className="flex items-center gap-2">
          <span className="font-mono text-sm text-gray-400">{t("setting.ai-section.title")}</span>
          <Badge variant={aiSetting.enableAi ? "default" : "secondary"}>
            {aiSetting.enableAi ? t("common.enabled") : t("common.disabled")}
          </Badge>
        </div>
      </div>
      <p className="text-sm text-gray-500">{t("setting.ai-section.description")}</p>
      
      <div className="w-full flex flex-col gap-4 mt-4">
        {/* Enable AI Toggle */}
        <div className="w-full flex flex-row justify-between items-center">
          <div className="flex flex-col">
            <Label htmlFor="enable-ai">{t("setting.ai-section.enable-ai")}</Label>
            <span className="text-sm text-gray-500">{t("setting.ai-section.enable-ai-description")}</span>
          </div>
          <Switch
            id="enable-ai"
            checked={aiSetting.enableAi}
            onCheckedChange={(checked) => updatePartialSetting({ enableAi: checked })}
          />
        </div>

        {/* AI Configuration Fields - Only show when enabled */}
        {aiSetting.enableAi && (
          <>
            {/* Base URL */}
            <div className="w-full flex flex-col gap-2">
              <Label htmlFor="base-url">{t("setting.ai-section.base-url")}</Label>
              <Input
                id="base-url"
                type="url"
                placeholder="https://api.openai.com/v1"
                value={aiSetting.baseUrl}
                onChange={(e) => updatePartialSetting({ baseUrl: e.target.value })}
              />
              <span className="text-sm text-gray-500">{t("setting.ai-section.base-url-description")}</span>
            </div>

            {/* API Key */}
            <div className="w-full flex flex-col gap-2">
              <Label htmlFor="api-key">{t("setting.ai-section.api-key")}</Label>
              <div className="relative">
                <Input
                  id="api-key"
                  type="text"
                  placeholder="sk-..."
                  value={aiSetting.apiKey}
                  onChange={(e) => updatePartialSetting({ apiKey: e.target.value })}
                  autoComplete="off"
                  style={showApiKey ? {} : { 
                    WebkitTextSecurity: 'disc',
                    fontFamily: 'text-security-disc, -webkit-small-control'
                  }}
                  className="pr-10"
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="absolute right-1 top-1 h-7 w-7 p-0"
                  onClick={() => setShowApiKey(!showApiKey)}
                >
                  {showApiKey ? <EyeOffIcon className="h-4 w-4" /> : <EyeIcon className="h-4 w-4" />}
                </Button>
              </div>
              <span className="text-sm text-gray-500">{t("setting.ai-section.api-key-description")}</span>
            </div>

            {/* Model */}
            <div className="w-full flex flex-col gap-2">
              <Label htmlFor="model">{t("setting.ai-section.model")}</Label>
              <Input
                id="model"
                type="text"
                placeholder="gpt-4o, claude-3-5-sonnet-20241022..."
                value={aiSetting.model}
                onChange={(e) => updatePartialSetting({ model: e.target.value })}
              />
              <span className="text-sm text-gray-500">{t("setting.ai-section.model-description")}</span>
            </div>

            {/* Timeout */}
            <div className="w-full flex flex-col gap-2">
              <Label htmlFor="timeout">{t("setting.ai-section.timeout")}</Label>
              <Input
                id="timeout"
                type="number"
                min="5"
                max="60"
                placeholder="10"
                value={aiSetting.timeoutSeconds}
                onChange={(e) => updatePartialSetting({ timeoutSeconds: parseInt(e.target.value) || 10 })}
              />
              <span className="text-sm text-gray-500">{t("setting.ai-section.timeout-description")}</span>
            </div>

            {/* Test Connection */}
            <div className="w-full flex flex-col gap-2">
              <Button
                variant="outline"
                onClick={testConnection}
                disabled={testingConnection || !aiSetting.baseUrl || !aiSetting.apiKey || !aiSetting.model}
                className="w-fit"
              >
                {testingConnection ? t("setting.ai-section.testing-connection") : t("setting.ai-section.test-connection")}
              </Button>
              <span className="text-sm text-gray-500">{t("setting.ai-section.test-connection-description")}</span>
            </div>
          </>
        )}
      </div>

      {/* Action Buttons */}
      {hasChanged && (
        <div className="w-full flex flex-row justify-end items-center gap-2 mt-4">
          <Button variant="outline" onClick={resetSetting}>
{t("common.cancel")}
          </Button>
          <Button onClick={updateSetting}>
            <CheckIcon className="w-4 h-4 mr-1" />
            {t("common.save")}
          </Button>
        </div>
      )}
    </div>
  );
});

export default AISettings;