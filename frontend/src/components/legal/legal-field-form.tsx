import { Plus, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import type { LegalColumn, LegalDocSpec, LegalField, LegalFieldType, LegalFieldValues } from "@/types/legal";

type Row = Record<string, unknown>;

/** Whether a field is shown for these values (show_when). */
export function isFieldVisible(field: LegalField, values: LegalFieldValues) {
  if (!field.show_when) {
    return true;
  }
  return String(values[field.show_when.field]) === String(field.show_when.equals);
}

function formatThousands(value: unknown) {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return "";
  }
  return new Intl.NumberFormat("id-ID").format(value);
}

function parseThousands(raw: string): number | null {
  const digits = raw.replace(/[^0-9]/g, "");
  if (!digits) {
    return null;
  }
  return Number(digits);
}

function newRow(columns: LegalColumn[]): Row {
  const row: Row = {};
  for (const column of columns) {
    if (column.default !== undefined && column.default !== null) {
      row[column.key] = column.default;
    }
  }
  return row;
}

interface ValueInputProps {
  id: string;
  type: LegalFieldType;
  value: unknown;
  onChange: (value: unknown) => void;
  disabled?: boolean;
  invalid?: boolean;
  placeholder?: string;
  options?: Array<{ value: string; label: string }>;
  label: string;
  compact?: boolean;
}

function ValueInput({ id, type, value, onChange, disabled, invalid, placeholder, options, label, compact }: ValueInputProps) {
  const common = {
    id,
    disabled,
    "aria-invalid": invalid || undefined,
    className: cn(compact ? "h-9 text-[13px]" : undefined),
  };
  switch (type) {
    case "textarea":
      return (
        <Textarea
          {...common}
          className={cn("min-h-[84px]")}
          onChange={(event) => onChange(event.target.value)}
          placeholder={placeholder}
          value={typeof value === "string" ? value : ""}
        />
      );
    case "date":
      return <Input {...common} onChange={(event) => onChange(event.target.value)} type="date" value={typeof value === "string" ? value : ""} />;
    case "money":
      return (
        <Input
          {...common}
          inputMode="numeric"
          onChange={(event) => onChange(parseThousands(event.target.value))}
          placeholder={placeholder ?? "0"}
          value={formatThousands(value)}
        />
      );
    case "number":
    case "percent":
      return (
        <Input
          {...common}
          inputMode="numeric"
          onChange={(event) => onChange(event.target.value === "" ? null : Number(event.target.value))}
          type="number"
          value={typeof value === "number" ? String(value) : ""}
        />
      );
    case "select":
      return (
        <Select
          aria-invalid={invalid || undefined}
          disabled={disabled}
          id={id}
          onValueChange={(next) => onChange(next)}
          options={options ?? []}
          placeholder="Pilih"
          triggerClassName={compact ? "h-9 text-[13px]" : undefined}
          value={typeof value === "string" ? value : ""}
        />
      );
    case "bool":
      return (
        <label className="flex h-10 cursor-pointer items-center gap-2 text-sm text-text-primary" htmlFor={id}>
          <input
            checked={value === true}
            className="h-4 w-4 accent-[color:var(--module-primary)]"
            disabled={disabled}
            id={id}
            onChange={(event) => onChange(event.target.checked)}
            type="checkbox"
          />
          {label}
        </label>
      );
    default:
      return (
        <Input
          {...common}
          onChange={(event) => onChange(event.target.value)}
          placeholder={placeholder}
          type={type === "email" ? "email" : "text"}
          value={typeof value === "string" ? value : ""}
        />
      );
  }
}

function ListField({
  field,
  rows,
  onChange,
  errors,
  disabled,
}: {
  field: LegalField;
  rows: Row[];
  onChange: (rows: Row[]) => void;
  errors: Record<string, string>;
  disabled?: boolean;
}) {
  const columns = field.columns ?? [];
  const update = (index: number, key: string, value: unknown) => {
    onChange(rows.map((row, rowIndex) => (rowIndex === index ? { ...row, [key]: value } : row)));
  };
  return (
    <div className="space-y-2">
      <div className="overflow-x-auto rounded-xl border border-border">
        <table className="w-full min-w-[560px] text-left text-[13px]">
          <thead className="bg-surface-muted text-[12px] text-text-secondary">
            <tr>
              <th className="w-10 px-3 py-2 font-medium">No</th>
              {columns.map((column) => (
                <th className="px-2 py-2 font-medium" key={column.key}>
                  {column.label}
                  {column.required ? <span className="text-error"> *</span> : null}
                </th>
              ))}
              <th className="w-10 px-2 py-2" />
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr>
                <td className="px-3 py-3 text-text-secondary" colSpan={columns.length + 2}>
                  Belum ada baris.
                </td>
              </tr>
            ) : null}
            {rows.map((row, index) => (
              <tr className="border-t border-border align-top" key={index}>
                <td className="px-3 py-2.5 text-text-secondary">{index + 1}</td>
                {columns.map((column) => {
                  const errorKey = `${field.key}.${index}.${column.key}`;
                  return (
                    <td className="px-2 py-1.5" key={column.key}>
                      <ValueInput
                        compact
                        disabled={disabled}
                        id={`${field.key}-${index}-${column.key}`}
                        invalid={Boolean(errors[errorKey])}
                        label={column.label}
                        onChange={(value) => update(index, column.key, value)}
                        options={column.options}
                        type={column.type}
                        value={row[column.key]}
                      />
                      {errors[errorKey] ? <p className="mt-1 text-[12px] text-error-strong">{errors[errorKey]}</p> : null}
                    </td>
                  );
                })}
                <td className="px-2 py-1.5">
                  <Button
                    aria-label={`Hapus baris ${index + 1}`}
                    disabled={disabled}
                    onClick={() => onChange(rows.filter((_, rowIndex) => rowIndex !== index))}
                    size="icon"
                    type="button"
                    variant="ghost"
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <Button disabled={disabled} onClick={() => onChange([...rows, newRow(columns)])} size="xs" type="button" variant="outline">
        <Plus className="h-4 w-4" />
        Tambah baris
      </Button>
    </div>
  );
}

/**
 * The form of one document type, built from the spec the API serves: fields
 * grouped by section, list fields as editable tables, hidden fields
 * (show_when) left out. Errors are keyed like the API's ("termin.0.nilai").
 */
export function LegalFieldForm({
  spec,
  values,
  onChange,
  errors,
  disabled,
}: {
  spec: LegalDocSpec;
  values: LegalFieldValues;
  onChange: (values: LegalFieldValues) => void;
  errors: Record<string, string>;
  disabled?: boolean;
}) {
  const sections: Array<{ name: string; fields: LegalField[] }> = [];
  for (const field of spec.fields) {
    if (!isFieldVisible(field, values)) {
      continue;
    }
    let section = sections.find((item) => item.name === field.section);
    if (!section) {
      section = { name: field.section, fields: [] };
      sections.push(section);
    }
    section.fields.push(field);
  }
  const set = (key: string, value: unknown) => onChange({ ...values, [key]: value });

  return (
    <div className="space-y-8">
      {sections.map((section) => (
        <section className="space-y-4" key={section.name}>
          <h3 className="border-b border-border pb-2 text-[15px] font-semibold text-text-primary">{section.name}</h3>
          <div className="grid gap-x-4 gap-y-5 md:grid-cols-2">
            {section.fields.map((field) => {
              const wide = field.type === "textarea" || field.type === "list";
              const error = errors[field.key];
              const id = `legal-field-${field.key}`;
              return (
                <div className={cn("space-y-1.5", wide && "md:col-span-2")} key={field.key}>
                  {field.type !== "bool" ? (
                    <label className="block text-[13px] font-medium text-text-secondary" htmlFor={id}>
                      {field.label}
                      {field.required ? <span className="text-error"> *</span> : null}
                    </label>
                  ) : null}
                  {field.type === "list" ? (
                    <ListField
                      disabled={disabled}
                      errors={errors}
                      field={field}
                      onChange={(rows) => set(field.key, rows)}
                      rows={Array.isArray(values[field.key]) ? (values[field.key] as Row[]) : []}
                    />
                  ) : (
                    <ValueInput
                      disabled={disabled}
                      id={id}
                      invalid={Boolean(error)}
                      label={field.label}
                      onChange={(value) => set(field.key, value)}
                      options={field.options}
                      placeholder={field.placeholder}
                      type={field.type}
                      value={values[field.key]}
                    />
                  )}
                  {error ? (
                    <p className="text-[12px] text-error-strong">{error}</p>
                  ) : field.help ? (
                    <p className="text-[12px] text-text-secondary">{field.help}</p>
                  ) : null}
                </div>
              );
            })}
          </div>
        </section>
      ))}
    </div>
  );
}
