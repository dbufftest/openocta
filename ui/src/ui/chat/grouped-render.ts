import { html, nothing } from "lit";
import { unsafeHTML } from "lit/directives/unsafe-html.js";
import type { AssistantIdentity } from "../assistant-identity.ts";
import type { MessageGroup, ToolCard } from "../types/chat-types.ts";
import { icons } from "../icons.ts";
import { toSanitizedMarkdownHtml } from "../markdown.ts";
import { renderCopyAsMarkdownButton } from "./copy-as-markdown.ts";
import {
  extractTextCached,
  extractThinkingCached,
  formatReasoningMarkdown,
} from "./message-extract.ts";
import { isToolResultMessage, normalizeRoleForGrouping } from "./message-normalizer.ts";
import { extractToolCards } from "./tool-cards.ts";

type ImageBlock = {
  url: string;
  alt?: string;
};

function extractDurationMs(message: unknown): number | null {
  const m = message as Record<string, unknown>;
  const candidates = [
    m.durationMs,
    m.elapsedMs,
    m.latencyMs,
    m.thinkingMs,
    (m.metrics as Record<string, unknown> | undefined)?.durationMs,
  ];
  for (const c of candidates) {
    if (typeof c === "number" && Number.isFinite(c) && c > 0) {
      return c;
    }
    if (typeof c === "string") {
      const n = Number(c);
      if (Number.isFinite(n) && n > 0) {
        return n;
      }
    }
  }
  return null;
}

function formatDurationShort(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return "";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(ms < 10000 ? 1 : 0)}s`;
  const mins = Math.floor(ms / 60000);
  const secs = Math.round((ms % 60000) / 1000);
  return `${mins}m${secs.toString().padStart(2, "0")}s`;
}

function groupElapsedMs(group: MessageGroup): number | null {
  const timestamps = group.messages
    .map((item) => {
      const m = item.message as Record<string, unknown>;
      return typeof m.timestamp === "number" ? m.timestamp : null;
    })
    .filter((value): value is number => value !== null);
  if (timestamps.length < 2) {
    return null;
  }
  const elapsed = Math.max(...timestamps) - Math.min(...timestamps);
  return elapsed > 0 ? elapsed : null;
}

function extractImages(message: unknown): ImageBlock[] {
  const m = message as Record<string, unknown>;
  const content = m.content;
  const images: ImageBlock[] = [];

  if (Array.isArray(content)) {
    for (const block of content) {
      if (typeof block !== "object" || block === null) {
        continue;
      }
      const b = block as Record<string, unknown>;

      if (b.type === "image") {
        // Handle source object format (from sendChatMessage)
        const source = b.source as Record<string, unknown> | undefined;
        if (source?.type === "base64" && typeof source.data === "string") {
          const data = source.data;
          const mediaType = (source.media_type as string) || "image/png";
          // If data is already a data URL, use it directly
          const url = data.startsWith("data:") ? data : `data:${mediaType};base64,${data}`;
          images.push({ url });
        } else if (typeof b.url === "string") {
          images.push({ url: b.url });
        }
      } else if (b.type === "image_url") {
        // OpenAI format
        const imageUrl = b.image_url as Record<string, unknown> | undefined;
        if (typeof imageUrl?.url === "string") {
          images.push({ url: imageUrl.url });
        }
      }
    }
  }

  return images;
}

export function renderReadingIndicatorGroup(assistant?: AssistantIdentity) {
  return html`
    <div class="chat-group assistant">
      ${renderAvatar("assistant", assistant)}
      <div class="chat-group-messages">
        <div class="chat-bubble chat-reading-indicator" aria-hidden="true">
          <span class="chat-reading-indicator__dots">
            <span></span><span></span><span></span>
          </span>
        </div>
      </div>
    </div>
  `;
}

export function renderStreamingGroup(
  text: string,
  startedAt: number,
  onOpenSidebar?: (content: string) => void,
  assistant?: AssistantIdentity,
) {
  const timestamp = new Date(startedAt).toLocaleTimeString([], {
    hour: "numeric",
    minute: "2-digit",
  });
  const name = assistant?.name ?? "Assistant";

  return html`
    <div class="chat-group assistant">
      ${renderAvatar("assistant", assistant)}
      <div class="chat-group-messages">
        ${renderGroupedMessage(
          {
            role: "assistant",
            content: [{ type: "text", text }],
            timestamp: startedAt,
          },
          { isStreaming: true, showReasoning: false, showToolTrace: true },
          onOpenSidebar,
        )}
        <div class="chat-group-footer">
          <span class="chat-sender-name">${name}</span>
          <span class="chat-group-timestamp">${timestamp}</span>
        </div>
      </div>
    </div>
  `;
}

export function renderMessageGroup(
  group: MessageGroup,
  opts: {
    onOpenSidebar?: (content: string) => void;
    showReasoning: boolean;
    showToolTrace: boolean;
    assistantName?: string;
    assistantAvatar?: string | null;
  },
) {
  const normalizedRole = normalizeRoleForGrouping(group.role);
  const assistantName = opts.assistantName ?? "Assistant";
  const who =
    normalizedRole === "user"
      ? "You"
      : normalizedRole === "assistant"
        ? assistantName
        : normalizedRole === "tool"
          ? "Tool"
          : normalizedRole;
  const roleClass =
    normalizedRole === "user" ? "user" : normalizedRole === "assistant" ? "assistant" : "other";
  const timestamp = new Date(group.timestamp).toLocaleTimeString([], {
    hour: "numeric",
    minute: "2-digit",
  });
  const durationMs =
    normalizedRole === "assistant"
      ? extractDurationMs(group.messages[group.messages.length - 1]?.message)
      : null;
  const durationLabel = durationMs ? formatDurationShort(durationMs) : "";

  return html`
    <div class="chat-group ${roleClass}">
      ${renderAvatar(group.role, {
        name: assistantName,
        avatar: opts.assistantAvatar ?? null,
      })}
      <div class="chat-group-messages">
        ${
          normalizedRole === "assistant"
            ? renderAssistantTurnMessages(group, opts)
            : group.messages.map((item, index) =>
                renderGroupedMessage(
                  item.message,
                  {
                    isStreaming: group.isStreaming && index === group.messages.length - 1,
                    showReasoning: opts.showReasoning,
                    showToolTrace: opts.showToolTrace,
                  },
                  opts.onOpenSidebar,
                ),
              )
        }
        <div class="chat-group-footer">
          <span class="chat-sender-name">${who}</span>
          <span class="chat-group-timestamp">${timestamp}</span>
          ${durationLabel
            ? html`<span class="chat-group-duration muted">思考 ${durationLabel}</span>`
            : nothing}
        </div>
      </div>
    </div>
  `;
}

function renderAvatar(role: string, assistant?: Pick<AssistantIdentity, "name" | "avatar">) {
  const normalized = normalizeRoleForGrouping(role);
  const assistantName = assistant?.name?.trim() || "Assistant";
  const assistantAvatar = assistant?.avatar?.trim() || "";
  const initial =
    normalized === "user"
      ? "U"
      : normalized === "assistant"
        ? assistantName.charAt(0).toUpperCase() || "A"
        : normalized === "tool"
          ? "⚙"
          : "?";
  const className =
    normalized === "user"
      ? "user"
      : normalized === "assistant"
        ? "assistant"
        : normalized === "tool"
          ? "tool"
          : "other";

  if (assistantAvatar && normalized === "assistant") {
    if (isAvatarUrl(assistantAvatar)) {
      return html`<img
        class="chat-avatar ${className}"
        src="${assistantAvatar}"
        alt="${assistantName}"
      />`;
    }
    return html`<div class="chat-avatar ${className}">${assistantAvatar}</div>`;
  }

  return html`<div class="chat-avatar ${className}">${initial}</div>`;
}

function isAvatarUrl(value: string): boolean {
  return (
    /^https?:\/\//i.test(value) || /^data:image\//i.test(value) || value.startsWith("/") // Relative paths from avatar endpoint
  );
}

function renderMessageImages(images: ImageBlock[]) {
  if (images.length === 0) {
    return nothing;
  }

  return html`
    <div class="chat-message-images">
      ${images.map(
        (img) => html`
          <img
            src=${img.url}
            alt=${img.alt ?? "Attached image"}
            class="chat-message-image"
            @click=${() => window.open(img.url, "_blank")}
          />
        `,
      )}
    </div>
  `;
}

/** Plain tool output only (no ### headings). Names appear on cards above; avoids a lone "Tool Output" title in the fold panel. */
function aggregateToolResultMarkdown(cards: ToolCard[]): string | null {
  const parts: string[] = [];
  for (const c of cards) {
    if (c.kind !== "result" || !c.text?.trim()) {
      continue;
    }
    parts.push(c.text.trim());
  }
  return parts.length > 0 ? parts.join("\n\n---\n\n") : null;
}

function collapseWhitespace(s: string): string {
  return s.replace(/\s+/g, " ").trim();
}

/** Plain text under ### tool headings (for deduping with extractText). */
function stripAggregatedToolHeadings(aggregated: string): string {
  return aggregated
    .split(/\n\n---\n\n/)
    .map((part) => part.replace(/^###[^\n]+\n+/, "").trim())
    .join("\n\n")
    .trim();
}

function combinedResultPlaintext(cards: ToolCard[]): string {
  return cards
    .filter((c) => c.kind === "result" && c.text?.trim())
    .map((c) => c.text!.trim())
    .join("\n\n");
}

/**
 * Prefer a single body: extractText often duplicates toolresult card text; avoid showing both.
 */
function mergeToolExpandableBody(markdown: string | null, cards: ToolCard[]): string | null {
  const md = (markdown ?? "").trim();
  const aggregated = aggregateToolResultMarkdown(cards)?.trim() ?? "";
  if (!md && !aggregated) {
    return null;
  }
  if (!aggregated) {
    return md || null;
  }
  if (!md) {
    return aggregated;
  }
  const combined = combinedResultPlaintext(cards);
  const strippedAgg = stripAggregatedToolHeadings(aggregated);
  const dupWithCards =
    combined &&
    (md === combined ||
      collapseWhitespace(md) === collapseWhitespace(combined) ||
      combined.includes(md) ||
      md.includes(combined));
  const dupWithAggShape =
    md === strippedAgg || collapseWhitespace(md) === collapseWhitespace(strippedAgg);
  if (dupWithCards || dupWithAggShape) {
    return aggregated;
  }
  return `${md}\n\n---\n\n${aggregated}`;
}

function toggleToolResultBody(e: Event) {
  const btn = e.currentTarget as HTMLButtonElement;
  if (btn.disabled) {
    return;
  }
  e.stopPropagation();
  const open = btn.getAttribute("aria-expanded") === "true";
  const next = !open;
  btn.setAttribute("aria-expanded", String(next));
  const block = btn.closest(".chat-tool-result-block");
  const body = block?.querySelector(".chat-tool-result-body") as HTMLElement | null;
  if (body) {
    body.hidden = !next;
  }
  block?.classList.toggle("chat-tool-result-block--open", next);
}


function toolCommandText(card: ToolCard): string {
  const args = card.args;
  if (args && typeof args === "object") {
    const rec = args as Record<string, unknown>;
    for (const key of ["command", "cmd", "script", "query", "path"]) {
      const value = rec[key];
      if (typeof value === "string" && value.trim()) {
        return value.trim();
      }
    }
  }
  if (typeof args === "string" && args.trim()) {
    return args.trim();
  }
  if (card.text?.trim()) {
    return card.text.trim().split(/\r?\n/, 1)[0] ?? "";
  }
  return card.name;
}

function extractToolOutputText(doc: string): string {
  const trimmed = doc.trim();
  if (!trimmed.startsWith("{")) {
    return doc;
  }
  try {
    const parsed = JSON.parse(trimmed) as Record<string, unknown>;
    if (typeof parsed.output === "string" && parsed.output.trim()) {
      return parsed.output;
    }
    const nested = parsed.data as Record<string, unknown> | undefined;
    if (typeof nested?.output === "string" && nested.output.trim()) {
      return nested.output;
    }
  } catch {
    return doc;
  }
  return doc;
}

function formatToolRunLabel(cards: ToolCard[]): string {
  const count = cards.filter((c) => c.kind === "call").length || cards.length;
  return count <= 1 ? "已运行命令" : `已运行 ${count} 条命令`;
}

function renderInlineToolCall(card: ToolCard) {
  const command = toolCommandText(card);
  const label = command ? `已运行 ${command}` : "已运行命令";
  return html`
    <div class="chat-tool-run chat-tool-run--call">
      <div class="chat-tool-run__summary">
        <span class="chat-tool-run__icon">${icons.wrench}</span>
        <span class="chat-tool-run__title">${label}</span>
        <span class="chat-tool-run__status">${icons.check}</span>
      </div>
    </div>
  `;
}

type ToolRunEntry = {
  command: string;
  tool: string;
  output: string;
  success: boolean;
};

function isToolResultLikeMessage(message: unknown): boolean {
  const m = message as Record<string, unknown>;
  const role = typeof m.role === "string" ? m.role.toLowerCase() : "";
  return (
    isToolResultMessage(message) ||
    role === "toolresult" ||
    role === "tool_result" ||
    typeof m.toolCallId === "string" ||
    typeof m.tool_call_id === "string"
  );
}

function inferToolSuccess(output: string): boolean {
  const normalized = output.toLowerCase();
  return !/(^|\n)\s*(error|failed|failure):|exit code:\s*[1-9]|exception|traceback/.test(
    normalized,
  );
}

function collectAssistantTurn(group: MessageGroup) {
  const runs: ToolRunEntry[] = [];
  const processText: string[] = [];
  let finalMessage: unknown | null = null;
  let currentRun: ToolRunEntry | null = null;

  for (const item of group.messages) {
    const message = item.message;
    const m = message as Record<string, unknown>;
    const role = typeof m.role === "string" ? m.role : "";
    const cards = extractToolCards(message);
    const text = extractTextCached(message)?.trim() ?? "";
    const isToolResult = isToolResultLikeMessage(message);
    const callCards = cards.filter((card) => card.kind === "call");

    if (callCards.length > 0) {
      for (const card of callCards) {
        currentRun = {
          command: toolCommandText(card),
          tool: card.name || "tool",
          output: "",
          success: true,
        };
        runs.push(currentRun);
      }
      if (text) {
        processText.push(text);
      }
      continue;
    }

    if (isToolResult) {
      const output = text ? extractToolOutputText(text) : "";
      if (currentRun) {
        currentRun.output = output;
        currentRun.success = inferToolSuccess(output);
      } else {
        runs.push({
          command: "command",
          tool: cards[0]?.name || "tool",
          output,
          success: inferToolSuccess(output),
        });
      }
      continue;
    }

    if (role.toLowerCase() === "assistant" && text) {
      finalMessage = message;
      if (runs.length > 0) {
        processText.push(text);
      }
    }
  }

  return { runs, processText, finalMessage };
}

function renderAssistantToolSummary(group: MessageGroup) {
  const { runs, processText } = collectAssistantTurn(group);
  if (runs.length === 0) {
    return nothing;
  }
  const durationMs =
    extractDurationMs(group.messages[group.messages.length - 1]?.message) ?? groupElapsedMs(group);
  const durationLabel = durationMs ? formatDurationShort(durationMs) : "";
  const toolCount = new Set(runs.map((run) => run.tool).filter(Boolean)).size || runs.length;
  const successCount = runs.filter((run) => run.success).length;

  return html`
    <details class="chat-turn-tools">
      <summary class="chat-turn-tools__summary">
        <span>已处理 ${durationLabel || "-"}</span>
        <span class="chat-turn-tools__chevron">${icons.chevronRight}</span>
      </summary>
      <div class="chat-turn-tools__body">
        <details class="chat-turn-tools__section">
          <summary class="chat-turn-tools__section-summary">
            <span>已运行 ${runs.length} 条命令 · 使用 ${toolCount} 个 tools</span>
            <span class="chat-turn-tools__chevron">${icons.chevronRight}</span>
          </summary>
          ${
            processText.length > 0
              ? html`<div class="chat-turn-tools__process">
                  ${processText.map((line) => html`<div>${line}</div>`)}
                </div>`
              : nothing
          }
          <details class="chat-turn-tools__section">
            <summary class="chat-turn-tools__section-summary">
              <span>命令列表 · 成功 ${successCount}/${runs.length}</span>
              <span class="chat-turn-tools__chevron">${icons.chevronRight}</span>
            </summary>
            <div class="chat-turn-tools__list">
              ${runs.map(
                (run) => html`
                  <details class="chat-turn-command">
                    <summary class="chat-turn-command__summary">
                      <span class="chat-turn-command__text">已运行 ${run.command || run.tool}</span>
                      <span class="chat-turn-command__status ${run.success ? "success" : "failed"}">
                        ${run.success ? "成功" : "失败"}
                      </span>
                    </summary>
                    <div class="chat-tool-run__panel">
                      <div class="chat-tool-run__panel-title">Shell</div>
                      <pre class="chat-tool-run__output">${run.output || "(no output)"}</pre>
                    </div>
                  </details>
                `,
              )}
            </div>
          </details>
        </details>
      </div>
    </details>
  `;
}

function renderAssistantTurnMessages(
  group: MessageGroup,
  opts: {
    onOpenSidebar?: (content: string) => void;
    showReasoning: boolean;
    showToolTrace: boolean;
  },
) {
  if (!opts.showToolTrace) {
    return group.messages.map((item, index) =>
      renderGroupedMessage(
        item.message,
        {
          isStreaming: group.isStreaming && index === group.messages.length - 1,
          showReasoning: opts.showReasoning,
          showToolTrace: opts.showToolTrace,
        },
        opts.onOpenSidebar,
      ),
    );
  }

  const turn = collectAssistantTurn(group);
  if (turn.runs.length === 0) {
    return group.messages.map((item, index) =>
      renderGroupedMessage(
        item.message,
        {
          isStreaming: group.isStreaming && index === group.messages.length - 1,
          showReasoning: opts.showReasoning,
          showToolTrace: opts.showToolTrace,
        },
        opts.onOpenSidebar,
      ),
    );
  }

  return html`
    ${renderAssistantToolSummary(group)}
    ${
      turn.finalMessage
        ? renderGroupedMessage(
            turn.finalMessage,
            {
              isStreaming: group.isStreaming,
              showReasoning: opts.showReasoning,
              showToolTrace: false,
            },
            opts.onOpenSidebar,
          )
        : nothing
    }
  `;
}

function renderCollapsedToolResult(
  toolCards: ToolCard[],
  images: ImageBlock[],
  markdown: string | null,
  reasoningMarkdown: string | null,
  opts: { isStreaming: boolean; showReasoning: boolean },
  _onOpenSidebar?: (content: string) => void,
) {
  const bodyDoc = mergeToolExpandableBody(markdown, toolCards);
  const primaryCommand =
    toolCards
      .filter((card) => card.kind === "call")
      .map(toolCommandText)
      .find(Boolean) ?? "";
  const runLabel = toolCards.length ? formatToolRunLabel(toolCards) : "已运行命令";
  const outputText = bodyDoc?.trim() ? extractToolOutputText(bodyDoc) : "";

  return html`
    <div class="chat-tool-result-block">
      <details class="chat-tool-run">
        <summary class="chat-tool-run__summary">
          <span class="chat-tool-run__icon">${icons.wrench}</span>
          <span class="chat-tool-run__title">${runLabel}</span>
          <span class="chat-tool-run__chevron">${icons.chevronRight}</span>
        </summary>
        ${
          primaryCommand
            ? html`<div class="chat-tool-run__command muted">已运行 ${primaryCommand}</div>`
            : nothing
        }
        ${
          opts.showReasoning && reasoningMarkdown
            ? html`
                <details class="chat-thinking" open>
                  <summary class="chat-thinking__summary">Reasoning</summary>
                  <div class="chat-thinking__content">
                    ${unsafeHTML(toSanitizedMarkdownHtml(reasoningMarkdown))}
                  </div>
                </details>
              `
            : nothing
        }
        ${renderMessageImages(images)}
        ${
          outputText
            ? html`
                <div class="chat-tool-run__panel">
                  <div class="chat-tool-run__panel-title">Shell</div>
                  <pre class="chat-tool-run__output">${outputText}</pre>
                </div>
              `
            : nothing
        }
      </details>
    </div>
  `;
}

function renderGroupedMessage(
  message: unknown,
  opts: { isStreaming: boolean; showReasoning: boolean; showToolTrace: boolean },
  onOpenSidebar?: (content: string) => void,
) {
  const m = message as Record<string, unknown>;
  const role = typeof m.role === "string" ? m.role : "unknown";
  const isToolResult =
    isToolResultMessage(message) ||
    role.toLowerCase() === "toolresult" ||
    role.toLowerCase() === "tool_result" ||
    typeof m.toolCallId === "string" ||
    typeof m.tool_call_id === "string";

  if (!opts.showToolTrace && isToolResult) {
    return nothing;
  }

  const toolCards = opts.showToolTrace ? extractToolCards(message) : [];
  const hasToolCards = toolCards.length > 0;
  const images = extractImages(message);
  const hasImages = images.length > 0;

  const extractedText = extractTextCached(message);
  const extractedThinking =
    opts.showReasoning && role === "assistant" ? extractThinkingCached(message) : null;
  const markdownBase = extractedText?.trim() ? extractedText : null;
  const reasoningMarkdown = extractedThinking ? formatReasoningMarkdown(extractedThinking) : null;
  const durationMs = role === "assistant" ? extractDurationMs(message) : null;
  const durationLabel = durationMs ? formatDurationShort(durationMs) : "";
  const markdown = markdownBase;
  const canCopyMarkdown = role === "assistant" && Boolean(markdown?.trim());

  const bubbleClasses = [
    "chat-bubble",
    canCopyMarkdown ? "has-copy" : "",
    opts.isStreaming ? "streaming" : "",
    "fade-in",
  ]
    .filter(Boolean)
    .join(" ");

  if (isToolResult) {
    return renderCollapsedToolResult(
      toolCards,
      images,
      markdown,
      reasoningMarkdown,
      opts,
      onOpenSidebar,
    );
  }

  if (!markdown && !hasToolCards && !hasImages) {
    return nothing;
  }

  return html`
    <div class="${bubbleClasses}">
      ${canCopyMarkdown ? renderCopyAsMarkdownButton(markdown!) : nothing}
      ${renderMessageImages(images)}
      ${
        reasoningMarkdown
          ? html`
              <details class="chat-thinking">
                <summary class="chat-thinking__summary">
                  思考过程${durationLabel ? html`<span class="muted"> · ${durationLabel}</span>` : nothing}
                </summary>
                <div class="chat-thinking__content">
                  ${unsafeHTML(toSanitizedMarkdownHtml(reasoningMarkdown))}
                </div>
              </details>
            `
          : nothing
      }
      ${
        markdown
          ? html`<div class="chat-text">${unsafeHTML(toSanitizedMarkdownHtml(markdown))}</div>`
          : nothing
      }
      ${opts.showToolTrace
        ? toolCards.filter((card) => card.kind === "call").map(renderInlineToolCall)
        : nothing}
    </div>
  `;
}
