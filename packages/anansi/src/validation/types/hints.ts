/**
 * hints.ts
 *
 * Defines type hints for generating form input controls based on schema field definitions.
 * Each hint type corresponds to a specific input control, providing metadata for code generation.
 */

/**
 * Hints for generating a file input control.
 */
export type FileHint = {
  type: "file";
  subtype: "video" | "audio" | "image" | "pdf" | "doc" | "txt"; // Required category for UI behavior
  label?: string;
  embed?: boolean; // True: base64-embed file; False/undefined: store as URL
  mimes?: string | string[]; // Optional MIME type constraints (e.g., "image/png")
  preview?: boolean; // True: File will be open for preview
  size?: {
    max?: number; // Max file size in bytes (e.g., 1048576 for 1MB)
    min?: number; // Min file size in bytes (e.g., 1024 for 1KB)
  };
  dimensions?: {
    width: number; //.credit Exact width in pixels (e.g., 800)
    height: number; // Exact height in pixels (e.g., 600)
  };
  group?: string; // Group identifier (e.g., "contactInfo")
  ignore?: boolean; // True: Skip input generation (e.g., for auto-generated values)
};

/**
 * Hints for generating a text-based input control.
 */
export type TextHint = {
  type: "text" | "email" | "tel" | "url" | "textarea";
  label?: string;
  placeholder?: string;
  group?: string; // Group identifier
  ignore?: boolean; // True: Skip input generation
};

/**
 * Hints for generating a secret input control (e.g., passwords, API keys).
 */
export type SecretHint = {
  type: "secret";
  label?: string;
  placeholder?: string;
  password?: boolean; // True: treat as password; False/undefined: generic secret
  group?: string; // Group identifier
  ignore?: boolean; // True: Skip input generation
};

/**
 * Hints for generating a number-based input control.
 */
export type NumberHint = {
  type: "number" | "range" | "integer" | "decimal";
  label?: string;
  step?: number;
  group?: string; // Group identifier
  ignore?: boolean; // True: Skip input generation
};

/**
 * Hints for generating a boolean input control.
 */
export type BooleanHint = {
  type: "checkbox" | "radio";
  label?: string;
  radioLabels?: { true: string; false: string };
  group?: string; // Group identifier
  ignore?: boolean; // True: Skip input generation
};

/**
 * Hints for generating an enum input control.
 */
export type EnumHint = {
  type: "select" | "radio";
  label?: string;
  group?: string;
  ignore?: boolean;
  options?: Array<{ value: string | number; label: string }>;
};

/**
 * Hints for generating an array input control.
 */
export type ArrayHint = {
  type: "list";
  label?: string;
  itemHint?: { type: string };
  group?: string; // Group identifier
  ignore?: boolean; // True: Skip input generation
};

/**
 * Hints for generating a set input control.
 */
export type SetHint = {
  type: "tags";
  label?: string;
  itemHint?: { type: string };
  group?: string; // Group identifier
  ignore?: boolean; // True: Skip input generation
};

/**
 * Hints for generating an object input control.
 */
export type ObjectHint = {
  type: "group";
  label?: string;
  collapsible?: boolean;
  group?: string; // Group identifier
  ignore?: boolean; // True: Skip input generation
};

/**
 * Hints for generating a date input control.
 */
export type DateHint = {
  type: "date" | "datetime" | "time"; // "date" for date-only, "datetime" for date and time, "time" for time-only
  label?: string;
  placeholder?: string; // Placeholder text (e.g., "YYYY-MM-DD")
  min?: string; // Minimum allowed date/time (ISO 8601 format, e.g., "2023-01-01")
  max?: string; // Maximum allowed date/time (ISO 8601 format, e.g., "2025-12-31")
  group?: string; // Group identifier (e.g., "eventDetails")
  ignore?: boolean; // True: Skip input generation
};

/**
 * Hints for generating a code input control (e.g., for code snippets or scripts).
 */
export type CodeHint = {
  type: "code";
  label?: string;
  language?: string; // Programming language (e.g., "javascript", "python"); flexible for custom languages
  placeholder?: string; // Placeholder text (e.g., "// Enter your code here")
  readonly?: boolean; // True: display-only; False/undefined: editable
  editorOptions?: {
    lineNumbers?: boolean; // Show line numbers in editor
    wordWrap?: boolean; // Enable word wrapping
    minimap?: boolean; // Show minimap (if supported by editor)
  }; // Optional editor-specific settings
  group?: string; // Group identifier (e.g., "scriptSettings")
  ignore?: boolean; // True: Skip input generation
};

/**
 * Union type for all possible input hints.
 * Note: DynamicHint removed to avoid overlap with TextHint; use TextHint for generic text needs.
 */
export type InputHint =
  | FileHint
  | TextHint
  | SecretHint
  | NumberHint
  | BooleanHint
  | EnumHint
  | ArrayHint
  | SetHint
  | ObjectHint
  | DateHint
  | CodeHint;

/**
 * Defines metadata for a group of inputs at the schema level.
 */
export type GroupDefinition = {
  name: string; // Matches hint.input.group (e.g., "contactInfo")
  label?: string; // Display label for the group (e.g., "Contact Information")
  description?: string; // Additional context (e.g., "User contact details")
};

/**
 * Defines hints at the schema level, including group metadata.
 */
export type SchemaHint = {
  groups?: GroupDefinition[]; // Array of group definitions
};
