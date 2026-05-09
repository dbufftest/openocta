import { gatewayHttpBase } from "../gateway-url.ts";
import type { GatewayBrowserClient } from "../gateway.ts";
import type { SkillStatusEntry, SkillStatusReport } from "../types.ts";

export type SkillsState = {
  client: GatewayBrowserClient | null;
  connected: boolean;
  skillsLoading: boolean;
  skillsReport: SkillStatusReport | null;
  skillsError: string | null;
  skillsBusyKey: string | null;
  skillEdits: Record<string, string>;
  skillMessages: SkillMessageMap;
  gatewayUrl?: string;
  token?: string;
  // Skill detail doc (SKILL.md content)
  skillsSkillDocContent?: string | null;
  skillsSkillDocLoading?: boolean;
  skillsSkillDocError?: string | null;
};

export type SkillMessage = {
  kind: "success" | "error";
  message: string;
};

export type SkillMessageMap = Record<string, SkillMessage>;

type LoadSkillsOptions = {
  clearMessages?: boolean;
};

function setSkillMessage(state: SkillsState, key: string, message?: SkillMessage) {
  if (!key.trim()) {
    return;
  }
  const next = { ...state.skillMessages };
  if (message) {
    next[key] = message;
  } else {
    delete next[key];
  }
  state.skillMessages = next;
}

function getErrorMessage(err: unknown) {
  if (err instanceof Error) {
    return err.message;
  }
  return String(err);
}

/**
 * Find workspace skill entry for a marketplace/catalog `folder` id.
 * Prefers skillKey match, then unique basename(baseDir), then unique name — avoids
 * treating another skill's display name as a folder id (false "many cards disabled").
 */
export function findSkillEntryForMarketFolder(
  report: SkillStatusReport | null | undefined,
  folder: string,
): SkillStatusEntry | undefined {
  const f = folder.trim();
  if (!f || !report?.skills?.length) return undefined;
  const skills = report.skills;
  const byKey = skills.find((e) => e.skillKey === f);
  if (byKey) return byKey;
  const baseMatches = skills.filter((e) => {
    const base = (e.baseDir ?? "").replace(/[/\\]+$/, "");
    const seg = base.split(/[/\\]/).pop() ?? "";
    return seg === f;
  });
  if (baseMatches.length === 1) return baseMatches[0];
  const nameMatches = skills.filter((e) => e.name === f);
  if (nameMatches.length === 1) return nameMatches[0];
  return undefined;
}

export function resolveSkillKeyForMarketFolder(
  report: SkillStatusReport | null | undefined,
  folder: string,
): string {
  return findSkillEntryForMarketFolder(report, folder)?.skillKey ?? folder.trim();
}

export function isMarketSkillDisabled(
  report: SkillStatusReport | null | undefined,
  folder: string,
): boolean {
  return findSkillEntryForMarketFolder(report, folder)?.disabled ?? false;
}

/** Which catalog folders should render as disabled (cards keyed by `folder`). */
export function disabledFoldersForMarketItems(
  report: SkillStatusReport | null | undefined,
  items: { folder?: string }[],
): Set<string> {
  if (!report?.skills?.length || !items.length) return new Set<string>();

  // Build lookup indexes once: O(m)
  const byKey = new Map<string, SkillStatusEntry>();
  const byBase = new Map<string, SkillStatusEntry[]>();
  const byName = new Map<string, SkillStatusEntry[]>();

  for (const e of report.skills) {
    byKey.set(e.skillKey, e);

    const base = (e.baseDir ?? "").replace(/[/\\]+$/, "").split(/[/\\]/).pop() ?? "";
    if (base) {
      const arr = byBase.get(base);
      if (arr) arr.push(e);
      else byBase.set(base, [e]);
    }

    if (e.name) {
      const arr = byName.get(e.name);
      if (arr) arr.push(e);
      else byName.set(e.name, [e]);
    }
  }

  // O(n) lookups using indexes
  const set = new Set<string>();
  for (const it of items) {
    const folder = (it.folder ?? "").trim();
    if (!folder) continue;

    const entry =
      byKey.get(folder) ??
      (byBase.get(folder)?.length === 1 ? byBase.get(folder)![0] : undefined) ??
      (byName.get(folder)?.length === 1 ? byName.get(folder)![0] : undefined);

    if (entry?.disabled) set.add(folder);
  }
  return set;
}

export async function loadSkills(state: SkillsState, options?: LoadSkillsOptions) {
  if (options?.clearMessages && Object.keys(state.skillMessages).length > 0) {
    state.skillMessages = {};
  }
  if (!state.client || !state.connected) {
    return;
  }
  if (state.skillsLoading) {
    return;
  }
  state.skillsLoading = true;
  state.skillsError = null;
  try {
    const res = await state.client.request<SkillStatusReport | undefined>("skills.status", {});
    if (res) {
      state.skillsReport = res;
    }
  } catch (err) {
    state.skillsError = getErrorMessage(err);
  } finally {
    state.skillsLoading = false;
  }
}

export function updateSkillEdit(state: SkillsState, skillKey: string, value: string) {
  state.skillEdits = { ...state.skillEdits, [skillKey]: value };
}

export async function updateSkillEnabled(state: SkillsState, skillKey: string, enabled: boolean) {
  if (!state.client || !state.connected) {
    state.skillsError = "无法连接网关，无法更新技能启用状态。请在 Overview 确认 Gateway 已连接。";
    return;
  }
  state.skillsBusyKey = skillKey;
  state.skillsError = null;
  try {
    await state.client.request("skills.update", { skillKey, enabled });
    await loadSkills(state);
    setSkillMessage(state, skillKey, {
      kind: "success",
      message: enabled ? "Skill enabled" : "Skill disabled",
    });
  } catch (err) {
    const message = getErrorMessage(err);
    state.skillsError = message;
    setSkillMessage(state, skillKey, {
      kind: "error",
      message,
    });
  } finally {
    state.skillsBusyKey = null;
  }
}

export async function saveSkillApiKey(state: SkillsState, skillKey: string) {
  if (!state.client || !state.connected) {
    return;
  }
  state.skillsBusyKey = skillKey;
  state.skillsError = null;
  try {
    const apiKey = state.skillEdits[skillKey] ?? "";
    await state.client.request("skills.update", { skillKey, apiKey });
    await loadSkills(state);
    setSkillMessage(state, skillKey, {
      kind: "success",
      message: "API key saved",
    });
  } catch (err) {
    const message = getErrorMessage(err);
    state.skillsError = message;
    setSkillMessage(state, skillKey, {
      kind: "error",
      message,
    });
  } finally {
    state.skillsBusyKey = null;
  }
}

export async function installSkill(
  state: SkillsState,
  skillKey: string,
  name: string,
  installId: string,
) {
  if (!state.client || !state.connected) {
    return;
  }
  state.skillsBusyKey = skillKey;
  state.skillsError = null;
  try {
    const result = await state.client.request<{ message?: string }>("skills.install", {
      name,
      installId,
      timeoutMs: 120000,
    });
    await loadSkills(state);
    setSkillMessage(state, skillKey, {
      kind: "success",
      message: result?.message ?? "Installed",
    });
  } catch (err) {
    const message = getErrorMessage(err);
    state.skillsError = message;
    setSkillMessage(state, skillKey, {
      kind: "error",
      message,
    });
  } finally {
    state.skillsBusyKey = null;
  }
}

export type SkillsUploadResult = {
  ok: boolean;
  error?: string;
  template?: string;
};

export async function uploadSkill(
  state: SkillsState,
  name: string,
  file: File,
): Promise<SkillsUploadResult> {
  const url = state.gatewayUrl ? gatewayHttpBase(state.gatewayUrl as string) : "";
  if (!url) {
    return { ok: false, error: "Gateway URL not configured" };
  }
  const form = new FormData();
  form.append("name", name.trim());
  form.append("file", file);
  const headers: Record<string, string> = {};
  if (state.token?.trim()) {
    headers["Authorization"] = `Bearer ${state.token.trim()}`;
  }
  try {
    const res = await fetch(`${url.replace(/\/$/, "")}/api/skills/upload`, {
      method: "POST",
      headers,
      body: form,
    });
    const data = (await res.json()) as { ok?: boolean; error?: string; template?: string };
    if (!res.ok) {
      const errMsg =
        res.status === 401
          ? "认证失败：网关令牌无效或未提供，请在 Overview 中配置正确的 Gateway Token"
          : (data.error ?? `Upload failed (${res.status})`);
      return { ok: false, error: errMsg, template: data.template };
    }
    return { ok: true };
  } catch (err) {
    const raw = err instanceof Error ? err.message : String(err);
    const msg = raw === "Failed to fetch" ? "网络请求失败，请检查网络连接" : raw;
    return { ok: false, error: msg };
  }
}

export async function deleteSkill(state: SkillsState, skillKey: string): Promise<void> {
  if (!state.client || !state.connected) {
    state.skillsError = "无法连接网关，无法删除技能。请在 Overview 确认 Gateway 已连接。";
    return;
  }
  state.skillsBusyKey = skillKey;
  state.skillsError = null;
  try {
    await state.client.request("skills.delete", { skillKey });
    await loadSkills(state, { clearMessages: true });
  } catch (err) {
    state.skillsError = getErrorMessage(err);
  } finally {
    state.skillsBusyKey = null;
  }
}

export async function loadSkillDoc(
  state: SkillsState,
  skillKey: string,
): Promise<void> {
  if (!state.client || !state.connected) {
    return;
  }
  state.skillsSkillDocLoading = true;
  state.skillsSkillDocError = null;
  state.skillsSkillDocContent = null;
  try {
    const res = await state.client.request<{ content?: string }>("skills.getDoc", {
      skillKey,
    });
    state.skillsSkillDocContent = res?.content ?? null;
  } catch (err) {
    state.skillsSkillDocError = getErrorMessage(err);
  } finally {
    state.skillsSkillDocLoading = false;
  }
}

export type SkillFileEntry = {
  path: string;
};

export async function listSkillFiles(
  state: SkillsState,
  skillKey: string,
): Promise<string[]> {
  if (!state.client || !state.connected) {
    return [];
  }
  try {
    const res = await state.client.request<{ files?: string[] }>("skills.listFiles", {
      skillKey,
    });
    return res?.files ?? [];
  } catch (err) {
    state.skillsError = getErrorMessage(err);
    return [];
  }
}

export async function getSkillFile(
  state: SkillsState,
  skillKey: string,
  filePath: string,
): Promise<string | null> {
  if (!state.client || !state.connected) {
    return null;
  }
  try {
    const res = await state.client.request<{ content?: string }>("skills.getFile", {
      skillKey,
      filePath,
    });
    return res?.content ?? null;
  } catch (err) {
    state.skillsError = getErrorMessage(err);
    return null;
  }
}

export async function saveSkillFile(
  state: SkillsState,
  skillKey: string,
  filePath: string,
  content: string,
): Promise<boolean> {
  if (!state.client || !state.connected) {
    return false;
  }
  state.skillsBusyKey = skillKey;
  state.skillsError = null;
  try {
    await state.client.request("skills.saveFile", {
      skillKey,
      filePath,
      content,
    });
    setSkillMessage(state, skillKey, {
      kind: "success",
      message: "文件保存成功",
    });
    return true;
  } catch (err) {
    const message = getErrorMessage(err);
    state.skillsError = message;
    setSkillMessage(state, skillKey, {
      kind: "error",
      message,
    });
    return false;
  } finally {
    state.skillsBusyKey = null;
  }
}
