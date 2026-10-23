"use client";

// What the reader decided about a thread in this browser: which threads are
// read, archived or removed from the view. The API stores labels per message
// and never shrinks a thread label list, so the list needs this local overlay
// to stay honest. Everything is scoped to one inbox address and kept in local
// storage; nothing secret is written.
//
// Every setter is idempotent: when nothing changes it returns the previous
// state, so an effect that marks a thread read cannot start a render loop.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

export type ViewState = {
  /** thread id to the ISO timestamp that was on screen when it was read. */
  read: Record<string, string>;
  archived: string[];
  hidden: string[];
};

const EMPTY: ViewState = { read: {}, archived: [], hidden: [] };
const STORAGE_PREFIX = "mai.webmail.v1.";

function storageKey(address: string): string {
  return `${STORAGE_PREFIX}${address.toLowerCase()}`;
}

function load(address: string): ViewState {
  if (typeof window === "undefined" || !address) return EMPTY;
  try {
    const raw = window.localStorage.getItem(storageKey(address));
    if (!raw) return EMPTY;
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null) return EMPTY;
    const value = parsed as Partial<ViewState>;
    return {
      read: typeof value.read === "object" && value.read !== null ? value.read : {},
      archived: Array.isArray(value.archived) ? value.archived.filter((id): id is string => typeof id === "string") : [],
      hidden: Array.isArray(value.hidden) ? value.hidden.filter((id): id is string => typeof id === "string") : [],
    };
  } catch {
    return EMPTY;
  }
}

function save(address: string, state: ViewState): void {
  if (typeof window === "undefined" || !address) return;
  try {
    window.localStorage.setItem(storageKey(address), JSON.stringify(state));
  } catch {
    // A browser that refuses storage still gets a working session; the
    // overlay just does not survive a reload.
  }
}

export type MailView = {
  isRead: (threadId: string, timestamp: string) => boolean;
  markRead: (threadId: string, timestamp: string) => void;
  markUnread: (threadId: string) => void;
  isArchived: (threadId: string) => boolean;
  setArchived: (threadId: string, archived: boolean) => void;
  isHidden: (threadId: string) => boolean;
  setHidden: (threadId: string, hidden: boolean) => void;
};

export function useMailView(address: string): MailView {
  const [state, setState] = useState<ViewState>(EMPTY);
  const addressRef = useRef(address);

  useEffect(() => {
    addressRef.current = address;
    setState(load(address));
  }, [address]);

  const update = useCallback((change: (current: ViewState) => ViewState) => {
    setState((current) => {
      const next = change(current);
      if (next === current) return current;
      save(addressRef.current, next);
      return next;
    });
  }, []);

  const markRead = useCallback(
    (threadId: string, timestamp: string) =>
      update((current) => {
        const seen = current.read[threadId];
        if (seen && new Date(seen).getTime() >= new Date(timestamp).getTime()) return current;
        return { ...current, read: { ...current.read, [threadId]: new Date().toISOString() } };
      }),
    [update],
  );

  const markUnread = useCallback(
    (threadId: string) =>
      update((current) => {
        if (!(threadId in current.read)) return current;
        const read = { ...current.read };
        delete read[threadId];
        return { ...current, read };
      }),
    [update],
  );

  const setArchived = useCallback(
    (threadId: string, archived: boolean) =>
      update((current) => {
        const isSet = current.archived.includes(threadId);
        if (isSet === archived) return current;
        return {
          ...current,
          archived: archived ? [...current.archived, threadId] : current.archived.filter((id) => id !== threadId),
        };
      }),
    [update],
  );

  const setHidden = useCallback(
    (threadId: string, hidden: boolean) =>
      update((current) => {
        const isSet = current.hidden.includes(threadId);
        if (isSet === hidden) return current;
        return {
          ...current,
          hidden: hidden ? [...current.hidden, threadId] : current.hidden.filter((id) => id !== threadId),
        };
      }),
    [update],
  );

  const isRead = useCallback(
    (threadId: string, timestamp: string) => {
      const seen = state.read[threadId];
      if (!seen) return false;
      return new Date(seen).getTime() >= new Date(timestamp).getTime();
    },
    [state.read],
  );

  const isArchived = useCallback((threadId: string) => state.archived.includes(threadId), [state.archived]);
  const isHidden = useCallback((threadId: string) => state.hidden.includes(threadId), [state.hidden]);

  return useMemo<MailView>(
    () => ({ isRead, markRead, markUnread, isArchived, setArchived, isHidden, setHidden }),
    [isRead, markRead, markUnread, isArchived, setArchived, isHidden, setHidden],
  );
}
