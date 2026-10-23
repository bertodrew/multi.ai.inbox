"use client";

// Loads one inbox page by page and folds the message metadata into thread rows.
// The API has no search parameter (see api_gaps), so a query is answered by
// walking older pages until enough rows match or the scan budget is spent;
// the list stays paginated either way through next_page_token.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { call, isAbort } from "@/lib/console/client";
import { MessageList, type Message } from "@/lib/console/schemas";
import type { ConsoleSession } from "@/lib/console/session";
import { groupThreads, matchesQuery, mergeMessages, tokensOf } from "./mail-util";
import type { ThreadSummary } from "./types";

const PAGE_SIZE = 50;
/** Rows a search tries to find before it stops reading older pages. */
const SEARCH_TARGET = 24;
/** Pages a search reads at most, so one query cannot hammer the API. */
const MAX_SCAN_PAGES = 8;

type LoadState = {
  key: string;
  messages: Message[];
  token: string;
  loading: boolean;
  scanning: boolean;
  loadingMore: boolean;
  error: unknown;
  moreError: unknown;
  pages: number;
};

const emptyState = (key: string): LoadState => ({
  key,
  messages: [],
  token: "",
  loading: true,
  scanning: false,
  loadingMore: false,
  error: undefined,
  moreError: undefined,
  pages: 0,
});

export type Mailbox = {
  threads: ThreadSummary[];
  messages: Message[];
  /** First page in flight: the list has nothing to show yet. */
  loading: boolean;
  /** A search is reading older pages in the background. */
  scanning: boolean;
  error: unknown;
  /** More pages exist: offer "Load older mail". */
  hasMore: boolean;
  loadingMore: boolean;
  moreError: unknown;
  loadMore: () => void;
  reload: () => void;
  /** Messages fetched so far, for the "searched N messages" hint. */
  scanned: number;
  searching: boolean;
};

export function useMailbox({
  session,
  inbox,
  query,
  label,
  includeReview,
  enabled,
}: {
  session: ConsoleSession;
  inbox: string;
  query: string;
  label: string;
  includeReview: boolean;
  enabled: boolean;
}): Mailbox {
  const key = `${inbox}|${label}|${includeReview ? "review" : "visible"}|${query}`;
  const [nonce, setNonce] = useState(0);
  const [state, setState] = useState<LoadState>(() => emptyState(key));
  const latest = useRef(state);
  latest.current = state;

  useEffect(() => {
    if (!enabled || !inbox) return;
    const ctrl = new AbortController();
    const tokens = tokensOf(query);
    setState(emptyState(key));

    void (async () => {
      let messages: Message[] = [];
      let token = "";
      let pages = 0;
      try {
        for (;;) {
          const page = await call(
            session,
            "GET /v0/inboxes/{inbox}/messages",
            {
              params: { inbox },
              query: {
                limit: PAGE_SIZE,
                label: label || undefined,
                include_review: includeReview || undefined,
                page_token: token || undefined,
              },
              signal: ctrl.signal,
            },
            MessageList,
          );
          messages = mergeMessages(messages, page.messages);
          pages += 1;
          token = page.next_page_token ?? "";
          if (ctrl.signal.aborted) return;

          const found = tokens.length === 0 ? 0 : groupThreads(messages).filter((row) => matchesQuery(row, tokens)).length;
          const scanning = tokens.length > 0 && found < SEARCH_TARGET && token !== "" && pages < MAX_SCAN_PAGES;
          setState({
            key,
            messages,
            token,
            loading: false,
            scanning,
            loadingMore: false,
            error: undefined,
            moreError: undefined,
            pages,
          });
          if (!scanning) break;
        }
      } catch (error) {
        if (ctrl.signal.aborted || isAbort(error)) return;
        setState({
          key,
          messages,
          token,
          loading: false,
          scanning: false,
          loadingMore: false,
          error,
          moreError: undefined,
          pages,
        });
      }
    })();

    return () => ctrl.abort();
  }, [key, nonce, enabled, inbox, label, includeReview, query, session]);

  const loadMore = useCallback(() => {
    const current = latest.current;
    if (!enabled || current.token === "" || current.loading || current.loadingMore || current.key !== key) return;
    setState((prev) => ({ ...prev, loadingMore: true, moreError: undefined }));
    void (async () => {
      try {
        const page = await call(
          session,
          "GET /v0/inboxes/{inbox}/messages",
          {
            params: { inbox },
            query: {
              limit: PAGE_SIZE,
              label: label || undefined,
              include_review: includeReview || undefined,
              page_token: current.token,
            },
          },
          MessageList,
        );
        setState((prev) =>
          prev.key !== key
            ? prev
            : {
                ...prev,
                messages: mergeMessages(prev.messages, page.messages),
                token: page.next_page_token ?? "",
                loadingMore: false,
                pages: prev.pages + 1,
              },
        );
      } catch (error) {
        if (isAbort(error)) return;
        setState((prev) => (prev.key !== key ? prev : { ...prev, loadingMore: false, moreError: error }));
      }
    })();
  }, [enabled, inbox, includeReview, key, label, session]);

  const reload = useCallback(() => setNonce((value) => value + 1), []);

  const empty = useMemo(() => emptyState(key), [key]);
  const current = state.key === key ? state : empty;
  const tokens = useMemo(() => tokensOf(query), [query]);
  const threads = useMemo(() => {
    const rows = groupThreads(current.messages);
    return tokens.length === 0 ? rows : rows.filter((row) => matchesQuery(row, tokens));
  }, [current.messages, tokens]);
  return {
    threads,
    messages: current.messages,
    loading: current.loading,
    scanning: current.scanning,
    error: current.error,
    hasMore: current.token !== "",
    loadingMore: current.loadingMore,
    moreError: current.moreError,
    loadMore,
    reload,
    scanned: current.messages.length,
    searching: query.trim() !== "",
  };
}
