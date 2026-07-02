"use client";

import "@glideapps/glide-data-grid/dist/index.css";

import { useCallback, useState } from "react";
import {
  DataEditor,
  GridCellKind,
  type EditableGridCell,
  type GridCell,
  type GridColumn,
  type Item,
} from "@glideapps/glide-data-grid";

import { useGlideTheme } from "./use-glide-theme";

const ROW_COUNT = 1000;

interface SpikeRow {
  id: number;
  name: string;
  email: string;
  company: string;
  age: number;
  score: number;
  active: boolean;
  notes: string;
}

const FIRST_NAMES = [
  "Ada",
  "Grace",
  "Alan",
  "Edsger",
  "Barbara",
  "Donald",
  "Margaret",
  "Linus",
  "Katherine",
  "Dennis",
];
const COMPANIES = [
  "Hivy",
  "Acme",
  "Globex",
  "Initech",
  "Umbrella",
  "Stark",
  "Wayne",
  "Hooli",
];

function makeRows(): SpikeRow[] {
  return Array.from({ length: ROW_COUNT }, (_, i) => {
    const first = FIRST_NAMES[i % FIRST_NAMES.length]!;
    const company = COMPANIES[i % COMPANIES.length]!;
    return {
      id: i + 1,
      name: `${first} ${String.fromCharCode(65 + (i % 26))}.`,
      email: `${first.toLowerCase()}${i}@${company.toLowerCase()}.com`,
      company,
      age: 21 + (i % 45),
      score: Math.round(((i * 37) % 1000) / 10) / 10,
      active: i % 3 !== 0,
      notes: `Row ${i + 1} generated for the React 19 spike`,
    };
  });
}

const INITIAL_COLUMNS: GridColumn[] = [
  { id: "id", title: "ID", width: 70 },
  { id: "name", title: "Name", width: 150 },
  { id: "email", title: "Email", width: 240 },
  { id: "company", title: "Company", width: 130 },
  { id: "age", title: "Age", width: 80 },
  { id: "score", title: "Score", width: 90 },
  { id: "active", title: "Active", width: 90 },
  { id: "notes", title: "Notes", width: 320 },
];

const COLUMN_KEYS = [
  "id",
  "name",
  "email",
  "company",
  "age",
  "score",
  "active",
  "notes",
] as const satisfies readonly (keyof SpikeRow)[];

export function GlideSpikeGrid() {
  const [rows, setRows] = useState<SpikeRow[]>(makeRows);
  const [columns, setColumns] = useState<GridColumn[]>(INITIAL_COLUMNS);
  const theme = useGlideTheme();

  const getCellContent = useCallback(
    ([col, row]: Item): GridCell => {
      const key = COLUMN_KEYS[col]!;
      const value = rows[row]![key];

      switch (key) {
        case "id":
        case "age":
        case "score":
          return {
            kind: GridCellKind.Number,
            data: value as number,
            displayData: String(value),
            allowOverlay: key !== "id",
            readonly: key === "id",
          };
        case "active":
          return {
            kind: GridCellKind.Boolean,
            data: value as boolean,
            allowOverlay: false,
            readonly: false,
          };
        default:
          return {
            kind: GridCellKind.Text,
            data: value as string,
            displayData: value as string,
            allowOverlay: true,
            readonly: false,
          };
      }
    },
    [rows],
  );

  const onCellEdited = useCallback(
    ([col, row]: Item, newValue: EditableGridCell) => {
      const key = COLUMN_KEYS[col]!;
      setRows((prev) => {
        const next = [...prev];
        const updated = { ...next[row]! };
        if (newValue.kind === GridCellKind.Text) {
          (updated[key] as string) = newValue.data;
        } else if (newValue.kind === GridCellKind.Number) {
          (updated[key] as number) = newValue.data ?? 0;
        } else if (newValue.kind === GridCellKind.Boolean) {
          (updated[key] as boolean) = newValue.data ?? false;
        }
        next[row] = updated;
        return next;
      });
    },
    [],
  );

  const onColumnResize = useCallback((column: GridColumn, newSize: number) => {
    setColumns((prev) =>
      prev.map((c) => (c.id === column.id ? { ...c, width: newSize } : c)),
    );
  }, []);

  return (
    <DataEditor
      columns={columns}
      rows={rows.length}
      getCellContent={getCellContent}
      onCellEdited={onCellEdited}
      onColumnResize={onColumnResize}
      theme={theme}
      rowMarkers="number"
      smoothScrollX
      smoothScrollY
      width="100%"
      height={640}
    />
  );
}
