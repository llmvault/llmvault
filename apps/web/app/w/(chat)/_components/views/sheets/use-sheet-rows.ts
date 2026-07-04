"use client"

import { useCallback, useEffect, useMemo, useRef } from "react"
import {
  keepPreviousData,
  useInfiniteQuery,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query"
import { toast } from "@heroui/react"
import { extractErrorMessage } from "@/lib/api/error"
import {
  deleteRows,
  insertRows,
  queryRows,
  rowsQuerySig,
  sheetKeys,
  updateRows,
  ROWS_PAGE_SIZE,
  type SheetFilterNode,
  type SheetRelationRef,
  type SheetRow,
  type SheetRowsQueryResponse,
  type SheetSort,
} from "@/app/w/(chat)/_lib/sheets"

/** Extra virtual rows shown as loading cells while the next window fetches. */
const LOADING_BUFFER = 60
/** Start fetching the next window this many rows before the loaded tail. */
const FETCH_AHEAD = 40

type RowsCache = InfiniteData<SheetRowsQueryResponse>

export interface SheetRowsController {
  rows: SheetRow[]
  relations: Record<string, SheetRelationRef>
  /** Row count handed to Glide (loaded rows + loading buffer). */
  gridRowCount: number
  isPending: boolean
  isError: boolean
  error: unknown
  refetch: () => void
  /** Prefetch the next window as the visible region approaches the tail. */
  onVisibleRowsChanged: (lastVisibleRow: number) => void
  /** Optimistic single-cell edit → PATCH rows (batch of one). */
  updateCell: (rowId: string, fieldId: string, value: unknown) => void
  /** Append an empty row; resolves to its index in the loaded window. */
  appendRow: () => Promise<number | undefined>
  deleteRowIds: (ids: string[]) => Promise<void>
}

export function useSheetRows({
  sheetId,
  pageId,
  filter,
  sorts,
  search,
}: {
  sheetId: string
  pageId: string
  filter?: SheetFilterNode
  sorts?: SheetSort[]
  search?: string
}): SheetRowsController {
  const queryClient = useQueryClient()
  const querySig = useMemo(
    () => rowsQuerySig({ filter, sorts, search }),
    [filter, sorts, search]
  )
  const queryKey = sheetKeys.rows(pageId, querySig)

  const query = useInfiniteQuery({
    queryKey,
    queryFn: ({ pageParam, signal }) =>
      queryRows(
        sheetId,
        pageId,
        {
          filter,
          sorts,
          search,
          cursor: pageParam || undefined,
          limit: ROWS_PAGE_SIZE,
          resolve_relations: true,
        },
        signal
      ),
    initialPageParam: "",
    getNextPageParam: (lastPage) => lastPage.next_cursor || undefined,
    placeholderData: keepPreviousData,
    refetchOnWindowFocus: true,
  })

  const rows = useMemo(
    () => query.data?.pages.flatMap((page) => page.rows ?? []) ?? [],
    [query.data]
  )
  const relations = useMemo(() => {
    const merged: Record<string, SheetRelationRef> = {}
    for (const page of query.data?.pages ?? []) {
      Object.assign(merged, page.relations ?? {})
    }
    return merged
  }, [query.data])

  const stateRef = useRef({
    rowCount: 0,
    hasNextPage: false,
    isFetchingNextPage: false,
    fetchNextPage: query.fetchNextPage,
  })
  useEffect(() => {
    stateRef.current = {
      rowCount: rows.length,
      hasNextPage: query.hasNextPage,
      isFetchingNextPage: query.isFetchingNextPage,
      fetchNextPage: query.fetchNextPage,
    }
  })

  const onVisibleRowsChanged = useCallback((lastVisibleRow: number) => {
    const state = stateRef.current
    if (
      state.hasNextPage &&
      !state.isFetchingNextPage &&
      lastVisibleRow >= state.rowCount - FETCH_AHEAD
    ) {
      void state.fetchNextPage()
    }
  }, [])

  const setCachedRows = useCallback(
    (mapRows: (rows: SheetRow[]) => SheetRow[]) => {
      queryClient.setQueryData<RowsCache>(queryKey, (data) => {
        if (!data?.pages) return data
        return {
          ...data,
          pages: data.pages.map((page) => ({
            ...page,
            rows: mapRows(page.rows ?? []),
          })),
        }
      })
    },
    [queryClient, queryKey]
  )

  const updateCell = useCallback(
    (rowId: string, fieldId: string, value: unknown) => {
      setCachedRows((cached) =>
        cached.map((row) =>
          row.id === rowId
            ? { ...row, data: { ...(row.data ?? {}), [fieldId]: value } }
            : row
        )
      )
      updateRows(sheetId, pageId, [
        { id: rowId, data: { [fieldId]: value } },
      ]).catch((error: unknown) => {
        toast.danger(extractErrorMessage(error, "Could not save the cell"))
        void queryClient.invalidateQueries({
          queryKey: sheetKeys.rowsPrefix(pageId),
        })
      })
    },
    [pageId, queryClient, setCachedRows, sheetId]
  )

  const appendRow = useCallback(async (): Promise<number | undefined> => {
    try {
      const created = await insertRows(sheetId, pageId, [{ data: {} }])
      const newRows = created.rows ?? []
      if (newRows.length > 0) {
        queryClient.setQueryData<RowsCache>(queryKey, (data) => {
          if (!data?.pages.length) return data
          const pages = [...data.pages]
          const last = pages[pages.length - 1]!
          pages[pages.length - 1] = {
            ...last,
            rows: [...(last.rows ?? []), ...newRows],
          }
          return { ...data, pages }
        })
      }
      void queryClient.invalidateQueries({
        queryKey: sheetKeys.structure(sheetId),
      })
      return stateRef.current.rowCount + newRows.length - 1
    } catch (error) {
      toast.danger(extractErrorMessage(error, "Could not add a row"))
      return undefined
    }
  }, [pageId, queryClient, queryKey, sheetId])

  const deleteRowIds = useCallback(
    async (ids: string[]) => {
      if (ids.length === 0) return
      const idSet = new Set(ids)
      setCachedRows((cached) =>
        cached.filter((row) => !row.id || !idSet.has(row.id))
      )
      try {
        await deleteRows(sheetId, pageId, ids)
        void queryClient.invalidateQueries({
          queryKey: sheetKeys.structure(sheetId),
        })
      } catch (error) {
        toast.danger(extractErrorMessage(error, "Could not delete rows"))
        void queryClient.invalidateQueries({
          queryKey: sheetKeys.rowsPrefix(pageId),
        })
      }
    },
    [pageId, queryClient, setCachedRows, sheetId]
  )

  return {
    rows,
    relations,
    gridRowCount: rows.length + (query.hasNextPage ? LOADING_BUFFER : 0),
    isPending: query.isPending,
    isError: query.isError,
    error: query.error,
    refetch: () => void query.refetch(),
    onVisibleRowsChanged,
    updateCell,
    appendRow,
    deleteRowIds,
  }
}
