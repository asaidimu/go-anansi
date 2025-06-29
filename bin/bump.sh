#!/bin/bash

# This script precisely replaces occurrences of an old Go module major version
# (e.g., "v5") with a new major version (e.g., "v6").
# It automatically detects the module path from go.mod and specifically targets
# the 'module' directive in go.mod files and import paths within .go files.
# This version uses POSIX-compliant tools and leverages Go tooling when available.

# --- Usage ---
# Save this script as, for example, `update-go-version.sh`.
# Make it executable: `chmod +x update-go-version.sh`
# Run it: `./update-go-version.sh <OLD_VERSION_NUMBER> <NEW_VERSION_NUMBER> [--dry-run]`
#
# Example: To change "v2" to "v3":
# `./update-go-version.sh 2 3`
#
# To dry-run and see what files would be affected (RECOMMENDED FIRST!):
# `./update-go-version.sh 2 3 --dry-run`

# --- Safety Precautions ---
# ALWAYS run with --dry-run first to see affected files.
# ALWAYS make a backup of your project before running this script, e.g.:
# `cp -R your_project_dir your_project_dir_backup`
# The script can create automatic backups with --backup flag.

# Exit immediately if a command exits with a non-zero status.
set -e

OLD_VERSION_NUM="$1"
NEW_VERSION_NUM="$2"
DRY_RUN=false
CREATE_BACKUP=false

# Process optional flags
shift 2 2>/dev/null || true  # Remove first two args, ignore errors if less than 2 args
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --backup)
      CREATE_BACKUP=true
      shift
      ;;
    *)
      echo "Unknown option: $1"
      echo "Valid options: --dry-run, --backup"
      exit 1
      ;;
  esac
done

# Function to perform sed replacement with proper escaping
perform_replacement() {
  local file="$1"
  local search_pattern="$2"
  local replace_pattern="$3"

  if sed --version >/dev/null 2>&1; then
    # GNU sed (Linux)
    sed -i "s|${search_pattern}|${replace_pattern}|g" "$file"
  else
    # BSD sed (macOS)
    sed -i '' "s|${search_pattern}|${replace_pattern}|g" "$file"
  fi
}

# Determine the correct sed in-place flag for portability (GNU vs. BSD/macOS)
if sed --version >/dev/null 2>&1; then
  # GNU sed (Linux)
  SED_INPLACE_FLAG="-i"
  SED_ERE_FLAG="-E"
else
  # BSD sed (macOS) - requires an empty string for no backup file
  SED_INPLACE_FLAG="-i ''"
  SED_ERE_FLAG="-E"
fi

# Function to display usage
show_usage() {
  echo "Usage: $0 <OLD_VERSION_NUMBER> <NEW_VERSION_NUMBER> [--dry-run] [--backup]"
  echo ""
  echo "Examples:"
  echo "  $0 2 3                    # Change v2 to v3"
  echo "  $0 2 3 --dry-run         # Preview changes without applying"
  echo "  $0 2 3 --backup          # Create backup before applying changes"
  echo "  $0 2 3 --dry-run --backup # Preview and prepare backup"
  echo ""
  echo "This script automatically detects your module path from go.mod"
}

# Validate arguments
if [ -z "$OLD_VERSION_NUM" ] || [ -z "$NEW_VERSION_NUM" ]; then
  show_usage
  exit 1
fi

# Validate that we're in a Go module directory
if [ ! -f "go.mod" ]; then
  echo "Error: go.mod file not found in current directory"
  echo "Please run this script from the root of your Go module"
  exit 1
fi

# Function to extract module path from go.mod
get_module_path_from_gomod() {
  # Extract the module path from the first line starting with "module "
  # Remove any existing version suffix (e.g., /v2, /v3, etc.)
  awk '/^module / {
    gsub(/^module[[:space:]]+/, "", $0)
    gsub(/\/v[0-9]+$/, "", $0)
    print $0
    exit
  }' go.mod
}

# Function to get current module path using Go tooling
get_module_path_go_list() {
  if command -v go >/dev/null 2>&1; then
    # Use go list to get the current module path, remove version suffix
    go list -m 2>/dev/null | sed -E 's/\/v[0-9]+$//' || echo ""
  else
    echo ""
  fi
}

# Detect module base path
echo "Detecting module path..."

# Try go list first (more reliable), fallback to parsing go.mod
MODULE_BASE_PATH=$(get_module_path_go_list)
if [ -z "$MODULE_BASE_PATH" ]; then
  echo "Go tooling not available or failed, parsing go.mod directly..."
  MODULE_BASE_PATH=$(get_module_path_from_gomod)
fi

if [ -z "$MODULE_BASE_PATH" ]; then
  echo "Error: Could not detect module path from go.mod"
  echo "Please ensure go.mod contains a valid 'module' directive"
  exit 1
fi

echo "Detected module path: $MODULE_BASE_PATH"

OLD_STRING="v${OLD_VERSION_NUM}"
NEW_STRING="v${NEW_VERSION_NUM}"

# Validate that old version exists in current module
CURRENT_MODULE_LINE=$(grep "^module " go.mod || echo "")
if [ -n "$CURRENT_MODULE_LINE" ] && ! echo "$CURRENT_MODULE_LINE" | grep -q "/${OLD_STRING}\$\|^module ${MODULE_BASE_PATH}\$"; then
  echo "Warning: Current go.mod doesn't seem to use version ${OLD_STRING}"
  echo "Current module line: $CURRENT_MODULE_LINE"
  echo "Expected to find: ${MODULE_BASE_PATH}/${OLD_STRING} or ${MODULE_BASE_PATH}"
  echo ""
  read -p "Continue anyway? (y/N): " -n 1 -r
  echo
  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted by user"
    exit 1
  fi
fi

echo "Attempting to replace '${MODULE_BASE_PATH}/${OLD_STRING}' with '${MODULE_BASE_PATH}/${NEW_STRING}' in .go and go.mod files."

# Create backup if requested
if [ "$CREATE_BACKUP" = true ]; then
  BACKUP_DIR="backup_$(date +%Y%m%d_%H%M%S)"
  echo "Creating backup in: $BACKUP_DIR"
  if [ "$DRY_RUN" = false ]; then
    mkdir -p "$BACKUP_DIR"
    find . -type f \( -name "*.go" -o -name "go.mod" \) -exec cp --parents {} "$BACKUP_DIR/" \; 2>/dev/null || \
    find . -type f \( -name "*.go" -o -name "go.mod" \) | while read -r file; do
      mkdir -p "$BACKUP_DIR/$(dirname "$file")"
      cp "$file" "$BACKUP_DIR/$file"
    done
    echo "Backup created successfully"
  else
    echo "Backup would be created in: $BACKUP_DIR (dry-run mode)"
  fi
fi

# Find files that would be affected
AFFECTED_FILES=$(find . -type f \( -name "*.go" -o -name "go.mod" \) -exec grep -l "${MODULE_BASE_PATH}/${OLD_STRING}" {} \; 2>/dev/null || true)

if [ -z "$AFFECTED_FILES" ]; then
  echo "No files found containing '${MODULE_BASE_PATH}/${OLD_STRING}'"
  echo "This might mean:"
  echo "  - The version ${OLD_STRING} is not currently used"
  echo "  - The module path detection was incorrect"
  echo "  - There are no import statements using the versioned path"
  exit 0
fi

if [ "$DRY_RUN" = true ]; then
  echo "--- DRY RUN MODE ---"
  echo "Files that would be affected:"
  echo "$AFFECTED_FILES"
  echo ""
  echo "Preview of changes:"
  echo "$AFFECTED_FILES" | while read -r file; do
    if [ -n "$file" ]; then
      echo "=== $file ==="
      # Show the lines that would change
      grep -n "${MODULE_BASE_PATH}/${OLD_STRING}" "$file" 2>/dev/null | head -5 | while read -r line; do
        echo "  BEFORE: $line"
        echo "  AFTER:  $(echo "$line" | sed "s|${MODULE_BASE_PATH}/${OLD_STRING}|${MODULE_BASE_PATH}/${NEW_STRING}|g")"
      done
      echo ""
    fi
  done
  echo "--------------------"
  echo "No changes were made. To apply changes, run without '--dry-run'."
else
  echo "--- APPLYING CHANGES ---"
  echo "Modifying files in place..."

  PROCESSED_COUNT=0
  echo "$AFFECTED_FILES" | while read -r file; do
    if [ -n "$file" ]; then
      echo "Processing: $file"

      # Specific replacement for go.mod 'module' directive
      if [[ "$file" == *"go.mod"* ]]; then
        # Replace module directive: "module path/vX" -> "module path/vY"
        perform_replacement "$file" "^module ${MODULE_BASE_PATH}/${OLD_STRING}" "module ${MODULE_BASE_PATH}/${NEW_STRING}"
        # Also handle case where there might be no version suffix currently
        perform_replacement "$file" "^module ${MODULE_BASE_PATH}$" "module ${MODULE_BASE_PATH}/${NEW_STRING}"
      fi

      # Specific replacement for .go import statements
      # Pattern 1: import "path/vX" -> import "path/vY"
      perform_replacement "$file" "import \"${MODULE_BASE_PATH}/${OLD_STRING}" "import \"${MODULE_BASE_PATH}/${NEW_STRING}"

      # Pattern 2: "path/vX" in grouped imports -> "path/vY" (with leading whitespace)
      perform_replacement "$file" "\"${MODULE_BASE_PATH}/${OLD_STRING}" "\"${MODULE_BASE_PATH}/${NEW_STRING}"

      # Pattern 3: import alias "path/vX" -> import alias "path/vY"
      # This will be caught by pattern 2 since we're replacing the quoted part

      PROCESSED_COUNT=$((PROCESSED_COUNT + 1))
    fi
  done

  echo "------------------------"
  echo "Replacement complete! Processed files with changes."
  echo ""
  echo "Next steps:"
  echo "1. Review the changes with: git diff"
  echo "2. Run: go mod tidy"
  echo "3. Test your code: go test ./..."
  echo "4. Update any documentation that references the old version"
fi
