import { html, nothing } from "lit";
import { t } from "../strings.js";
import type { SecurityConfigForm } from "../controllers/security.ts";
import type { ApprovalsListResult } from "../controllers/approvals.ts";
import type { Tab } from "../navigation.ts";
import { renderApprovals } from "./approvals.ts";

export type SecurityProps = {
  security: SecurityConfigForm | null;
  saving: boolean;
  pendingApprovalsCount: number;
  onPresetApply: (preset: "off" | "loose" | "standard" | "strict") => void;
  onPatch: (path: string[], value: unknown) => void;
  onSave: () => void;
  pathForTab: (tab: Tab) => string;
  approvalsLoading: boolean;
  approvalsResult: ApprovalsListResult | null;
  approvalsError: string | null;
  onApprovalsRefresh: () => void;
  onApprove: (requestId: string) => void;
  onDeny: (requestId: string, reason?: string) => void;
  onWhitelistSession: (requestId: string) => void;
};

function ensureArray<T>(arr: T[] | undefined): T[] {
  return Array.isArray(arr) ? arr : [];
}

function joinLines(arr: string[]): string {
  return ensureArray(arr).filter(Boolean).join("\n");
}

function splitLines(s: string): string[] {
  return (s || "")
    .split("\n")
    .map((x) => x.trim())
    .filter(Boolean);
}

function formatApprovalTime(ms: number | undefined): string {
  if (ms == null) return "—";
  return new Date(ms).toLocaleString();
}

const RESOURCE_PRESETS: { label: string; bytes: number }[] = [
  { label: "512M", bytes: 512 * 1024 * 1024 },
  { label: "1G", bytes: 1024 * 1024 * 1024 },
  { label: "2G", bytes: 2 * 1024 * 1024 * 1024 },
  { label: "4G", bytes: 4 * 1024 * 1024 * 1024 },
];

export function renderSecurity(props: SecurityProps) {
  const s = props.security ?? {};
  const sandbox = s.sandbox ?? {};
  const commandPolicy = s.commandPolicy ?? {};
  const approvalQueue = s.approvalQueue ?? {};

  const sandboxEnabled = sandbox.enabled === true;
  const commandPolicyEnabled = commandPolicy.enabled === true;
  const approvalEnabled = approvalQueue.enabled === true;

  const allowedPaths = joinLines(ensureArray(sandbox.allowedPaths));
  const networkAllow = joinLines(ensureArray(sandbox.networkAllow));
  const resourceLimit = sandbox.resourceLimit ?? {};
  const maxCpuPercent = resourceLimit.maxCpuPercent ?? "";
  const maxMemoryBytes = resourceLimit.maxMemoryBytes ?? "";
  const maxDiskBytes = resourceLimit.maxDiskBytes ?? "";

  const defaultPolicy = commandPolicy.defaultPolicy ?? "ask";
  const denyLines = ensureArray(commandPolicy.deny);
  const askLines = ensureArray(commandPolicy.ask);
  const allowLines = ensureArray(commandPolicy.allow);
  const denyRulesText = joinLines(denyLines);
  const askRulesText = joinLines(askLines);
  const allowRulesText = joinLines(allowLines);

  function patchRulesFromTextareas(denyText: string, askText: string, allowText: string) {
    props.onPatch(["commandPolicy", "deny"], splitLines(denyText));
    props.onPatch(["commandPolicy", "ask"], splitLines(askText));
    props.onPatch(["commandPolicy", "allow"], splitLines(allowText));
  }
  const banArguments = joinLines(ensureArray(commandPolicy.banArguments));
  const maxLength = commandPolicy.maxLength ?? "";
  const secretPatterns = joinLines(ensureArray(commandPolicy.secretPatterns));

  const approvalTimeoutSeconds = approvalQueue.timeoutSeconds ?? "";
  const approvalBlockOnApproval = approvalQueue.blockOnApproval !== false;

  const presetLabel = (p: string) => {
    switch (p) {
      case "off":
        return t("securityPresetOff");
      case "loose":
        return t("securityPresetLoose");
      case "standard":
        return t("securityPresetStandard");
      case "strict":
        return t("securityPresetStrict");
      default:
        return p;
    }
  };

  return html`
    <section class="card">
      <div class="row" style="justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px;">
        <div>
          <div class="card-title">${t("navTitleSandbox")}</div>
          <div class="card-sub">${t("subtitleSandbox")}</div>
        </div>
      </div>

      <!-- Overview card -->
      <div class="security-overview-card" style="margin-top: 20px; padding: 20px; background: var(--bg-content, #f8fafc); border-radius: 12px; border: 1px solid var(--border, #e2e8f0);">
        <div class="card-sub" style="margin-bottom: 12px; font-size: 14px;">${t("securityOverviewTitle")}</div>
        <div class="row" style="flex-wrap: wrap; gap: 24px;">
          <div>
            <div class="muted" style="font-size: 12px;">${t("securityOverviewPreset")}</div>
            <div style="font-size: 16px; font-weight: 600;">${presetLabel(s.preset ?? "standard")}</div>
          </div>
          <div>
            <div class="muted" style="font-size: 12px;">${t("securityOverviewSandbox")}</div>
            <div style="font-size: 16px; font-weight: 600; color: ${sandboxEnabled ? "var(--accent, #16a34a)" : "var(--text-secondary)"};">${sandboxEnabled ? t("sandboxEnabled") : t("sandboxDisabled")}</div>
          </div>
          <div>
            <div class="muted" style="font-size: 12px;">${t("securityOverviewCommandPolicy")}</div>
            <div style="font-size: 16px; font-weight: 600; color: ${commandPolicyEnabled ? "var(--accent, #16a34a)" : "var(--text-secondary)"};">${commandPolicyEnabled ? t("sandboxEnabled") : t("sandboxDisabled")}</div>
          </div>
          <div>
            <div class="muted" style="font-size: 12px;">${t("securityOverviewPendingApprovals")}</div>
            <div style="font-size: 16px; font-weight: 600; color: ${props.pendingApprovalsCount > 0 ? "var(--danger, #dc2626)" : "var(--text-secondary)"};">${props.pendingApprovalsCount}</div>
          </div>
        </div>
      </div>

      <!-- Quick presets -->
      <div style="margin-top: 20px;">
        <div class="card-sub" style="margin-bottom: 8px; font-size: 14px;">${t("securityPresetsTitle")}</div>
        <div class="muted" style="font-size: 13px; margin-bottom: 12px;">${t("securityPresetsHint")}</div>
        <div class="row" style="flex-wrap: wrap; gap: 12px; margin-bottom: 8px;">
          ${(["off", "loose", "standard", "strict"] as const).map(
            (preset) => html`
              <button
                type="button"
                class="btn ${s.preset === preset ? "primary" : ""}"
                ?disabled=${props.saving}
                @click=${() => props.onPresetApply(preset)}
              >
                ${presetLabel(preset)}
              </button>
            `,
          )}
        </div>
        <div class="muted" style="font-size: 12px; line-height: 1.5;">
          <div style="margin-bottom: 4px;"><strong>${presetLabel("off")}</strong>：${t("securityPresetOffDesc")}</div>
          <div style="margin-bottom: 4px;"><strong>${presetLabel("loose")}</strong>：${t("securityPresetLooseDesc")}</div>
          <div style="margin-bottom: 4px;"><strong>${presetLabel("standard")}</strong>：${t("securityPresetStandardDesc")}</div>
          <div><strong>${presetLabel("strict")}</strong>：${t("securityPresetStrictDesc")}</div>
        </div>
      </div>

      <div class="sandbox-sections" style="margin-top: 24px;">
        <!-- Environment boundary (collapsed by default) -->
        <details class="sandbox-details">
          <summary class="sandbox-summary">
            <span>${t("securitySectionSandbox")}</span>
            <span class="security-help" title=${t("securitySectionSandboxDesc")}>❕</span>
          </summary>
          <div class="sandbox-section-body" style="margin-top: 16px;">
            <div class="muted" style="font-size: 13px; margin-bottom: 12px;">${t("securitySectionSandboxDesc")}</div>
            <div class="row" style="align-items: center; gap: 12px; margin-bottom: 16px;">
              <button type="button" class="btn ${sandboxEnabled ? "btn-ok" : ""}" ?disabled=${props.saving} @click=${() => props.onPatch(["sandbox", "enabled"], !sandboxEnabled)}>
                ${sandboxEnabled ? t("sandboxActionDisable") : t("sandboxActionEnable")}
              </button>
              <span class="muted" style="font-size: 13px;">${sandboxEnabled ? t("sandboxEnabled") : t("sandboxDisabled")}</span>
            </div>
            <div class="sandbox-form-center">
              <div class="field" style="width: 100%; margin-bottom: 16px;">
                <span>${t("sandboxAllowedPaths")}</span>
                <span class="textarea"><textarea rows="4" .value=${allowedPaths} placeholder="/tmp&#10;./workspace" @input=${(e: Event) => props.onPatch(["sandbox", "allowedPaths"], splitLines((e.target as HTMLTextAreaElement).value))}></textarea></span>
              </div>
              <div class="field" style="width: 100%; margin-bottom: 16px;">
                <span>${t("sandboxNetworkAllow")}</span>
                <span class="textarea"><textarea rows="4" .value=${networkAllow} placeholder="localhost&#10;127.0.0.1" @input=${(e: Event) => props.onPatch(["sandbox", "networkAllow"], splitLines((e.target as HTMLTextAreaElement).value))}></textarea></span>
              </div>
              <div style="margin: 24px 0;">
                <div class="card-sub" style="margin-bottom: 12px; font-size: 14px;">${t("sandboxResourceLimit")}</div>
                <div class="row" style="flex-wrap: wrap; gap: 8px; margin-bottom: 12px;">
                  ${RESOURCE_PRESETS.map(
                    (r) => html`
                      <button
                        type="button"
                        class="btn btn--sm"
                        @click=${() => {
                          props.onPatch(["sandbox", "resourceLimit", "maxMemoryBytes"], r.bytes);
                          props.onPatch(["sandbox", "resourceLimit", "maxDiskBytes"], r.bytes);
                        }}
                      >
                        ${r.label}
                      </button>
                    `,
                  )}
                  <span class="muted" style="font-size: 13px; align-self: center;">${t("securityResourceCustom")}</span>
                </div>
                <div class="row" style="flex-wrap: wrap; gap: 12px;">
                  <div class="field" style="flex: 1 1 120px; min-width: 0;">
                    <span style="font-size: 14px;">${t("sandboxMaxCPUPercent")}</span>
                    <span class="input"><input type="text" .value=${String(maxCpuPercent)} placeholder="60" @input=${(e: Event) => props.onPatch(["sandbox", "resourceLimit", "maxCpuPercent"], parseNum((e.target as HTMLInputElement).value))} /></span>
                  </div>
                  <div class="field" style="flex: 1 1 160px; min-width: 0;">
                    <span style="font-size: 14px;">${t("sandboxMaxMemoryBytes")}</span>
                    <span class="input"><input type="text" .value=${String(maxMemoryBytes)} placeholder="1G" @input=${(e: Event) => props.onPatch(["sandbox", "resourceLimit", "maxMemoryBytes"], (e.target as HTMLInputElement).value.trim() || undefined)} /></span>
                  </div>
                  <div class="field" style="flex: 1 1 160px; min-width: 0;">
                    <span style="font-size: 14px;">${t("sandboxMaxDiskBytes")}</span>
                    <span class="input"><input type="text" .value=${String(maxDiskBytes)} placeholder="1G" @input=${(e: Event) => props.onPatch(["sandbox", "resourceLimit", "maxDiskBytes"], (e.target as HTMLInputElement).value.trim() || undefined)} /></span>
                  </div>
                </div>
              </div>
            </div>
            <div class="row" style="gap: 8px; margin-top: 16px;">
              <button type="button" class="btn primary" ?disabled=${props.saving} @click=${props.onSave}>${props.saving ? t("commonLoading") : t("commonSave")}</button>
            </div>
          </div>
        </details>

        <!-- Command policy -->
        <details class="sandbox-details" style="margin-top: 16px;">
          <summary class="sandbox-summary">
            <span>${t("securitySectionCommandPolicy")}</span>
            <span class="security-help" title=${t("securitySectionCommandPolicyDesc")}>❕</span>
          </summary>
          <div class="sandbox-section-body" style="margin-top: 16px;">
            <div class="muted" style="font-size: 13px; margin-bottom: 12px;">${t("securitySectionCommandPolicyDesc")}</div>
            <div class="row" style="align-items: center; gap: 12px; margin-bottom: 16px;">
              <button type="button" class="btn ${commandPolicyEnabled ? "btn-ok" : ""}" ?disabled=${props.saving} @click=${() => props.onPatch(["commandPolicy", "enabled"], !commandPolicyEnabled)}>
                ${commandPolicyEnabled ? t("sandboxActionDisable") : t("sandboxActionEnable")}
              </button>
              <span class="muted" style="font-size: 13px;">${commandPolicyEnabled ? t("sandboxEnabled") : t("sandboxDisabled")}</span>
            </div>
            <div class="field" style="margin-bottom: 16px;">
              <span style="font-size: 14px;">${t("securityDefaultPolicy")}</span>
              <div class="row" style="gap: 16px; margin-top: 8px;">
                ${(["deny", "ask", "allow"] as const).map(
                  (p) => html`
                    <label class="row" style="align-items: center; gap: 6px; cursor: pointer;">
                      <span class="radio"><input type="radio" name="defaultPolicy" .checked=${defaultPolicy === p} @change=${() => props.onPatch(["commandPolicy", "defaultPolicy"], p)} /></span>
                      <span>${p === "deny" ? t("securityDefaultDeny") : p === "ask" ? t("securityDefaultAsk") : t("securityDefaultAllow")}</span>
                    </label>
                  `,
                )}
              </div>
            </div>
            <div class="card-sub" style="margin-bottom: 8px; font-size: 14px;">${t("securityRulesList")}</div>
            <div class="muted" style="font-size: 12px; margin-bottom: 12px;">${t("securityRulesHint")}</div>
            <div class="field" style="margin-bottom: 16px;">
              <span style="font-size: 14px;">${t("securityActionDeny")}</span>
              <span class="textarea"><textarea rows="4" .value=${denyRulesText} placeholder="sudo&#10;dd&#10;mkfs&#10;rm -rf" @input=${(e: Event) => patchRulesFromTextareas((e.target as HTMLTextAreaElement).value, askRulesText, allowRulesText)}></textarea></span>
              <div class="muted" style="font-size: 12px; margin-top: 4px;">${t("securityRulesDenyHint")}</div>
            </div>
            <div class="field" style="margin-bottom: 16px;">
              <span style="font-size: 14px;">${t("securityActionAsk")}</span>
              <span class="textarea"><textarea rows="4" .value=${askRulesText} placeholder="rm&#10;mv&#10;cp" @input=${(e: Event) => patchRulesFromTextareas(denyRulesText, (e.target as HTMLTextAreaElement).value, allowRulesText)}></textarea></span>
              <div class="muted" style="font-size: 12px; margin-top: 4px;">${t("securityRulesAskHint")}</div>
            </div>
            <div class="field" style="margin-bottom: 16px;">
              <span style="font-size: 14px;">${t("securityActionAllow")}</span>
              <span class="textarea"><textarea rows="4" .value=${allowRulesText} placeholder="ls&#10;pwd&#10;echo" @input=${(e: Event) => patchRulesFromTextareas(denyRulesText, askRulesText, (e.target as HTMLTextAreaElement).value)}></textarea></span>
              <div class="muted" style="font-size: 12px; margin-top: 4px;">${t("securityRulesAllowHint")}</div>
            </div>
            <details style="margin-top: 16px;">
              <summary class="muted" style="font-size: 13px; cursor: pointer;">${t("securityAdvancedOptions")}</summary>
              <div style="margin-top: 12px;">
                <div class="field" style="margin-bottom: 12px;">
                  <span style="font-size: 14px;">${t("sandboxBanArguments")}</span>
                  <span class="textarea"><textarea rows="4" .value=${banArguments} placeholder="--no-preserve-root&#10;/dev/" @input=${(e: Event) => props.onPatch(["commandPolicy", "banArguments"], splitLines((e.target as HTMLTextAreaElement).value))}></textarea></span>
                </div>
                <div class="field" style="margin-bottom: 12px;">
                  <span style="font-size: 14px;">${t("securityMaxLength")}</span>
                  <span class="input"><input type="text" .value=${String(maxLength)} placeholder="4096" @input=${(e: Event) => props.onPatch(["commandPolicy", "maxLength"], parseNum((e.target as HTMLInputElement).value))} /></span>
                </div>
                <div class="field">
                  <span style="font-size: 14px;">${t("sandboxSecretPatterns")}</span>
                  <span class="textarea"><textarea rows="4" style="font-family: var(--mono);" .value=${secretPatterns} placeholder="sk-[a-zA-Z0-9]{48}" @input=${(e: Event) => props.onPatch(["commandPolicy", "secretPatterns"], splitLines((e.target as HTMLTextAreaElement).value))}></textarea></span>
                </div>
              </div>
            </details>
            <div class="row" style="gap: 8px; margin-top: 16px;">
              <button type="button" class="btn primary" ?disabled=${props.saving} @click=${props.onSave}>${props.saving ? t("commonLoading") : t("commonSave")}</button>
            </div>
          </div>
        </details>

        <!-- Approval settings -->
        <details class="sandbox-details" style="margin-top: 16px;">
          <summary class="sandbox-summary">
            <span>${t("securitySectionApprovalQueue")}</span>
            <span class="security-help" title=${t("securitySectionApprovalQueueDesc")}>❕</span>
          </summary>
          <div class="sandbox-section-body" style="margin-top: 16px;">
            <div class="muted" style="font-size: 13px; margin-bottom: 12px;">${t("securitySectionApprovalQueueDesc")}</div>
            <div class="row" style="align-items: center; gap: 12px; margin-bottom: 16px;">
              <button type="button" class="btn ${approvalEnabled ? "btn-ok" : ""}" ?disabled=${props.saving} @click=${() => props.onPatch(["approvalQueue", "enabled"], !approvalEnabled)}>
                ${approvalEnabled ? t("sandboxActionDisable") : t("sandboxActionEnable")}
              </button>
              <span class="muted" style="font-size: 13px;">${approvalEnabled ? t("sandboxEnabled") : t("sandboxDisabled")}</span>
            </div>
            ${approvalEnabled
              ? html`
                  <div class="row" style="align-items: flex-start; gap: 8px; margin-bottom: 16px;">
                    <span class="checkbox"><input type="checkbox" id="blockOnApproval" .checked=${approvalBlockOnApproval} ?disabled=${props.saving} @input=${(e: Event) => props.onPatch(["approvalQueue", "blockOnApproval"], (e.target as HTMLInputElement).checked)} /></span>
                    <label for="blockOnApproval" style="font-size: 14px; cursor: pointer;">${t("securityApprovalBlockOnApproval")} <span class="muted" style="font-size: 12px;">${t("securityApprovalBlockOnApprovalHint")}</span></label>
                  </div>
                  <div class="field" style="margin-bottom: 16px;">
                    <span style="font-size: 14px;">${t("securityApprovalTimeoutSeconds")}</span>
                    <span class="input"><input type="text" .value=${String(approvalTimeoutSeconds)} placeholder="300" @input=${(e: Event) => props.onPatch(["approvalQueue", "timeoutSeconds"], parseNum((e.target as HTMLInputElement).value))} /></span>
                    <div class="muted" style="font-size: 12px; margin-top: 4px;">${t("securityApprovalTimeoutSecondsHint")}</div>
                  </div>
                `
              : nothing}
            <div class="row" style="gap: 8px; margin-bottom: 20px;">
              <button type="button" class="btn primary" ?disabled=${props.saving} @click=${props.onSave}>${props.saving ? t("commonLoading") : t("commonSave")}</button>
            </div>

            <!-- Approval queue list (full: pending, approved, denied, whitelisted) -->
            ${renderApprovals({
              approvalsLoading: props.approvalsLoading,
              approvalsResult: props.approvalsResult,
              approvalsError: props.approvalsError,
              onApprovalsRefresh: props.onApprovalsRefresh,
              onApprove: props.onApprove,
              onDeny: props.onDeny,
              onWhitelistSession: props.onWhitelistSession,
              pathForTab: props.pathForTab,
            })}
          </div>
        </details>
      </div>
    </section>
  `;
}

function parseNum(s: string): number | undefined {
  const v = parseInt(s.trim(), 10);
  return Number.isNaN(v) ? undefined : v;
}
