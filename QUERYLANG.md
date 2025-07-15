# Natural Language Query Grammar Specification

## 1. Introduction

This document specifies a natural language-like grammar for constructing queries that maps directly to a structured JSON Query DSL. The grammar provides a concise, intuitive, and human-readable way to express complex data retrieval operations while maintaining unambiguous mapping to the underlying data model.

## 2. Core Query Structure

A query is composed of optional clauses. A query can contain either a `WHERE` clause or a `SEARCH` clause, but not both.

```
[WHERE <filters> | SEARCH <text_search_config>]
[SORT BY <sort_config>]
[PAGINATE <pagination_config>]
[INCLUDE <projection_inclusion>]
[EXCLUDE <projection_exclusion>]
[COMPUTE <computed_fields>]
[JOIN <join_config>]
[AGGREGATE <aggregation_config>]
[GROUP BY <grouping_fields>]
[HAVING <having_filters>]
[HINT <hint_config>]
```

**Case Sensitivity Rules:**

  - Keywords (WHERE, AND, INCLUDE, AS, etc.): Case-insensitive
  - Identifiers (field names, function names, aliases): Case-sensitive
  - String literals: Case-sensitive and enclosed in double quotes

## 3. Data Types and Literals

  - **Strings**: Enclosed in double quotes: `"electronics"`, `"P90D"`
  - **Numbers**: Standard numeric values: `100`, `500.5`, `-42.7`
  - **Booleans**: `true`, `false`
  - **Null**: `null`
  - **Arrays**: `[1, 2, 3]`, `["a", "b", "c"]`
  - **Objects**: `{"key": "value", "nested": {"field": 123}}`

## 4. Identifiers and Field Paths

  - **Identifiers**: Letters, numbers, and underscores, starting with a letter. Case-sensitive.
  - **Field Paths**: Dot notation for nested fields: `address.city`, `user.profile.settings.theme`
  - **Array Access**: Bracket notation: `items[0]`, `tags[*]` (for all elements)

## 5. Filters (WHERE Clause)

**Purpose**: Filter records based on field values, logical combinations, function results, or text search.
**JSON Mapping**: `QueryDSL.filters`

### 5.1 Basic Conditions

**Syntax**: `<field_path> <comparison_operator> <value>`

### 5.2 Comparison Operators

| Symbol | Meaning | JSON Operator | Example |
|---|---|---|---|
| `==` | Equal to | `eq` | `status == "active"` |
| `!=` | Not equal to | `neq` | `status != "inactive"` |
| `<` | Less than | `lt` | `price < 100` |
| `<=` | Less than or equal to | `lte` | `quantity <= 50` |
| `>` | Greater than | `gt` | `views > 1000` |
| `>=` | Greater than or equal to | `gte` | `rating >= 4.5` |
| `IN` | Included in a set | `in` | `tag IN ("new", "featured")` |
| `NOT IN` | Not included in a set | `nin` | `country NOT IN ("US", "CA")` |
| `CONTAINS` | Field contains value | `contains` | `description CONTAINS "waterproof"` |
| `NOT CONTAINS` | Field does not contain value | `ncontains` | `tags NOT CONTAINS "deprecated"` |
| `EXISTS` | Field exists | `exists` | `optionalField EXISTS` |
| `NOT EXISTS` | Field does not exist | `nexists` | `deletedAt NOT EXISTS` |

### 5.3 Logical Operators

**Syntax**:

  - `AND(<condition1>, <condition2>, ...)`
  - `OR(<condition1>, <condition2>, ...)`
  - `XOR(<condition1>, <condition2>, ...)`
  - `NOR(<condition1>, <condition2>, ...)`
  - `NOT(<condition>)`

### 5.4 Function Calls as Conditions

Boolean-returning functions can be used directly as conditions:

  - `IS_HIGH_RISK_CUSTOMER(customerId)`
  - `IS_WEEKEND(orderDate)`

### 5.5 Subqueries

**Syntax**: `<field> IN (<inner_query>)`

**Example**:

```
WHERE userId IN (
  WHERE orderDate > DATE_SUB(CURRENT_DATE(), "P30D") 
  INCLUDE customerId
)
```

## 6. Text Search (SEARCH Clause)

**Purpose**: Full-text search across specified fields.
**JSON Mapping**: `QueryDSL.filters.TextMatch`

**Syntax**: `  <search_type> SEARCH <query_text> [IN (<field_list>)] [WITH (<search_options>)] `

### 6.1 Search Types

  - `MATCH`: Standard full-text search (default)
  - `PHRASE`: Exact phrase matching
  - `PREFIX`: Prefix matching
  - `WILDCARD`: Wildcard matching
  - `FUZZY`: Fuzzy matching with edit distance
  - `REGEX`: Regular expression matching

### 6.3 Search Options

  - `FUZZINESS <number>`: Edit distance for fuzzy matching (0-2)
  - `MINIMUM_MATCH <percentage>`: Minimum matching criteria
  - `BOOST <field_boosts>`: Per-field relevance boosts
  - `ANALYZER <analyzer_name>`: Text analyzer to use
  - `OPERATOR <text_operator>`: Text Operator

#### 6.3.1 Text Operators

  - `AND`: All terms must match
  - `OR`: Any term can match

**Note**: If the `OPERATOR` option is not specified, the default behavior is `OR`.

**Example**:

```
FUZZY SEARCH "premium wireless headphones" 
IN (title, description, tags) 
WITH (
  OPERATOR AND,
  FUZZINESS 1, 
  MINIMUM_MATCH "75%", 
  BOOST {"title": 2.0, "description": 1.5} 
)
```

## 7. Sorting (SORT BY Clause)

**Purpose**: Specify result ordering.
**JSON Mapping**: `QueryDSL.sort`

**Syntax**: `SORT BY <field_path> [ASC | DESC] {, <field_path> [ASC | DESC]}*`

**Example**: `SORT BY price DESC, name ASC, createdAt DESC`

## 8. Pagination (PAGINATE Clause)

**Purpose**: Control result set size and navigation.
**JSON Mapping**: `QueryDSL.pagination`

### 8.1 Offset-based Pagination

**Syntax**: `PAGINATE OFFSET <number> LIMIT <number>`

**Example**: `PAGINATE OFFSET 20 LIMIT 10`

### 8.2 Cursor-based Pagination

**Syntax**: `PAGINATE CURSOR <string_literal> LIMIT <number> [FORWARD | BACKWARD]`

**Example**: `PAGINATE CURSOR "eyJpZCI6MTIzfQ==" LIMIT 25 FORWARD`

## 9. Projections (Data Selection and Shaping)

**Purpose**: Select, exclude, or create fields in query results.
**JSON Mapping**: `QueryDSL.projection`

### 9.1 Include Fields (INCLUDE Clause)

**Syntax**: `INCLUDE <projection_item> {, <projection_item>}*`

**Projection Items**:

  - `<field_path>`: Simple field (`firstName`, `address.city`)
  - `<parent_field> { <nested_projection> }`: Nested object projection

**Examples**:

```
INCLUDE name, price, details.weight
INCLUDE customerDetails { firstName, lastName, contactInfo { email, phone } }
```

### 9.2 Exclude Fields (EXCLUDE Clause)

**Syntax**: `EXCLUDE <projection_item> {, <projection_item>}*`

**Example**: `EXCLUDE passwordHash, internalNotes, userSettings { debugMode, telemetryEnabled }`

### 9.3 Computed Fields (COMPUTE Clause)

**Syntax**: `COMPUTE <computed_field> {, <computed_field>}*`

**Computed Field Types**:

  - `<function_call> AS <alias>`
  - `<case_expression> AS <alias>`

**Examples**:

```
COMPUTE 
  DATE_SUB(manufactureDate, "P1Y") AS warrantyEndDate,
  CASE 
    WHEN price > 1000 THEN "Expensive" 
    WHEN price > 200 THEN "Mid-range" 
    ELSE "Affordable" 
  END AS priceCategory
```

## 10. Joins (JOIN Clause)

**Purpose**: Combine data from related collections.
**JSON Mapping**: `QueryDSL.joins`

**Syntax**: `JOIN <join_type> <relation_name> AS <alias> ON <join_condition> [<join_projection>]`

### 10.1 Join Types

  - `INNER`: Inner join (default)
  - `LEFT`: Left outer join
  - `RIGHT`: Right outer join
  - `FULL`: Full outer join

### 10.2 Join Projection

You can optionally project the fields of the joined relation. This projection follows the same syntax as the main query's `INCLUDE`, `EXCLUDE`, and `COMPUTE` clauses. If no projection is specified, all fields from the joined relation are returned.

**Example**:

```
JOIN LEFT orders AS customerOrders 
ON customerOrders.customerId == id 
WHERE customerOrders.orderDate > DATE_SUB(CURRENT_DATE(), "P90D")
INCLUDE orderId, totalAmount, items { productId, quantity }
```

## 11. Aggregations (AGGREGATE Clause)

**Purpose**: Perform summary calculations on data.
**JSON Mapping**: `QueryDSL.aggregations`

When used with a `GROUP BY` clause, functions are calculated for each group. If `GROUP BY` is omitted, aggregation functions are calculated over the entire result set.

**Syntax**: `AGGREGATE <aggregation_function>(<field_path>) AS <alias> {, <aggregation_function>(<field_path>) AS <alias>}*`

### 11.1 Aggregation Functions

  - `COUNT`: Count records
  - `SUM`: Sum numeric values
  - `AVG`: Average numeric values
  - `MIN`: Minimum value
  - `MAX`: Maximum value

**Example**: `AGGREGATE COUNT(*) AS totalCustomers, SUM(orderValue) AS totalRevenue, AVG(rating) AS avgRating`

## 12. Grouping (GROUP BY Clause)

**Purpose**: Group records for aggregation.
**JSON Mapping**: `QueryDSL.groupBy`

**Syntax**: `GROUP BY <field_path> {, <field_path>}*`

**Example**: `GROUP BY region, customerType`

## 13. Having Filters (HAVING Clause)

**Purpose**: Filter grouped results after aggregation.
**JSON Mapping**: `QueryDSL.having`

**Syntax**: `HAVING <filter_expression>`

Uses the same syntax as WHERE clause but operates on aggregated results.

**Example**: `HAVING COUNT(*) > 10 AND AVG(orderValue) > 500`

## 14. Query Hints (HINT Clause)

**Purpose**: Provide optimization directives.
**JSON Mapping**: `QueryDSL.hints`

**Syntax**: `HINT <hint_type> [<hint_value>]`

### 14.1 Hint Types

  - `USE INDEX <index_name>`: Suggest using specific index
  - `FORCE INDEX <index_name>`: Force using specific index
  - `NO INDEX`: Disable index usage
  - `MAX_TIME <seconds>`: Set execution timeout

**Example**: `HINT USE INDEX idx_customer_region, MAX_TIME 60`

## 15. Functions

**Syntax**: `FUNCTION_NAME(arg1, arg2, ...)`

Arguments can be:

  - Literal values
  - Field paths
  - Other function calls (nesting allowed)
  - Arrays and objects

**Examples**:

```
DATE_SUB(CURRENT_DATE(), "P90D")
CONCAT(firstName, " ", lastName)
CALCULATE_DISTANCE(address.coordinates, [40.7128, -74.0060])
```

## 16. Case Expressions

**Syntax**:

```
CASE 
  WHEN <condition1> THEN <value1>
  WHEN <condition2> THEN <value2>
  ...
  [ELSE <default_value>]
END
```

**Example**:

```
CASE 
  WHEN age < 18 THEN "Minor"
  WHEN age >= 65 THEN "Senior"
  ELSE "Adult"
END
```

## 17. Implicit Behaviors and Defaults

  - **Default Projection**: All top-level fields if no INCLUDE/EXCLUDE specified
  - **Default Sort Direction**: ASC if not specified
  - **Default Pagination**: All matching records (or system default limit)
  - **Default Join Type**: INNER if not specified
  - **Default Text Search Type**: MATCH if not specified
  - **Default Text Operator**: OR if not specified
  - **Boolean Function Calls**: Implicitly compared to `true`

## 18. Backus-Naur Form (BNF)

```bnf
<query> ::=
    (<where_clause> | <search_clause>)?
    [<sort_clause>]
    [<paginate_clause>]
    [<projection_clause>]
    [<join_clause>]
    [<aggregate_clause>]
    [<group_by_clause>]
    [<having_clause>]
    [<hint_clause>]

<where_clause> ::=
    "WHERE" <filter_expression>

<filter_expression> ::=
    <filter_condition>
    | <logical_function>
    | <function_call>

<logical_function> ::=
    "AND" "(" <filter_expression> | <search_clause>  {"," <filter_expression>}* ")"
    | "OR" "(" <filter_expression> | <search_clause>  {"," <filter_expression>}* ")"
    | "XOR" "(" <filter_expression> {"," <filter_expression>}* ")"
    | "NOR" "(" <filter_expression> {"," <filter_expression>}* ")"
    | "NOT" "(" <filter_expression> ")"

<search_clause> ::=
    <search_type> "SEARCH" <string_literal> 
    ["IN" "(" <field_list> ")"]
    ["WITH" "(" <search_options> ")"]

<filter_condition> ::=
    <field_path> <comparison_operator> <filter_value>
    | <field_path> "EXISTS"
    | <field_path> "NOT EXISTS"

<comparison_operator> ::=
    "==" | "!=" | "<" | "<=" | ">" | ">="
    | "IN" | "NOT IN"
    | "CONTAINS" | "NOT CONTAINS"

<filter_value> ::=
    <literal_value>
    | <field_path>
    | <function_call>
    | <array_literal>
    | <object_literal>
    | <subquery>

<subquery> ::=
    "(" <query> ")"

<function_call> ::=
    <identifier> "(" [<function_arguments>] ")"

<function_arguments> ::=
    <filter_value> {"," <filter_value>}*

<sort_clause> ::=
    "SORT BY" <sort_item> {"," <sort_item>}*

<sort_item> ::=
    <field_path> [<sort_direction>]

<sort_direction> ::=
    "ASC" | "DESC"

<paginate_clause> ::=
    "PAGINATE" <pagination_type>

<pagination_type> ::=
    "OFFSET" <number_literal> "LIMIT" <number_literal>
    | "CURSOR" <string_literal> "LIMIT" <number_literal> ["FORWARD" | "BACKWARD"]

<projection_clause> ::=
    [<include_clause>]
    [<exclude_clause>]
    [<compute_clause>]

<include_clause> ::=
    "INCLUDE" <projection_item> {"," <projection_item>}*

<exclude_clause> ::=
    "EXCLUDE" <projection_item> {"," <projection_item>}*

<projection_item> ::=
    <field_path>
    | <identifier> "{" <nested_projection> "}"

<nested_projection> ::=
    [<include_clause>] [<exclude_clause>] [<compute_clause>]

<compute_clause> ::=
    "COMPUTE" <computed_field> {"," <computed_field>}*

<computed_field> ::=
    <function_call> "AS" <identifier>
    | <case_expression> "AS" <identifier>

<case_expression> ::=
    "CASE" {<case_when_then>}+ ["ELSE" <filter_value>] "END"

<case_when_then> ::=
    "WHEN" <filter_expression> "THEN" <filter_value>

<join_clause> ::=
    {"JOIN" [<join_type>] <identifier> "AS" <identifier> "ON" <filter_expression> [<nested_projection>]}+

<join_type> ::=
    "INNER" | "LEFT" | "RIGHT" | "FULL"

<aggregate_clause> ::=
    "AGGREGATE" <aggregation_item> {"," <aggregation_item>}*

<aggregation_item> ::=
    <aggregation_function> "(" <field_path> ")" "AS" <identifier>

<aggregation_function> ::=
    "COUNT" | "SUM" | "AVG" | "MIN" | "MAX"

<group_by_clause> ::=
    "GROUP BY" <field_path> {"," <field_path>}*

<having_clause> ::=
    "HAVING" <filter_expression>

<hint_clause> ::=
    "HINT" <hint_item> {"," <hint_item>}*

<hint_item> ::=
    "USE INDEX" <identifier>
    | "FORCE INDEX" <identifier>
    | "NO INDEX"
    | "MAX_TIME" <number_literal>

<search_type> ::=
    "MATCH" | "PHRASE" | "PREFIX" | "WILDCARD" | "FUZZY" | "REGEX"

<text_operator> ::=
    "AND" | "OR"

<search_options> ::=
    <search_option> {"," <search_option>}*

<search_option> ::=
    "FUZZINESS" <number_literal>
    | "MINIMUM_MATCH" <string_literal>
    | "BOOST" <object_literal>
    | "ANALYZER" <string_literal>
    | "OPERATOR" <text_operator>

<field_path> ::=
    <identifier> {"." <identifier>}* ["[" <array_index> "]"]

<array_index> ::=
    <number_literal> | "*"

<field_list> ::=
    <field_path> {"," <field_path>}*

<array_literal> ::=
    "[" [<literal_value> {"," <literal_value>}*] "]"

<object_literal> ::=
    "{" [<object_pair> {"," <object_pair>}*] "}"

<object_pair> ::=
    <string_literal> ":" <literal_value>

<literal_value> ::=
    <string_literal> | <number_literal> | <boolean_literal> | "null"

<identifier> ::= 
    [a-zA-Z_][a-zA-Z0-9_]*

<string_literal> ::= 
    "." [^"]* "."

<number_literal> ::= 
    [-]?[0-9]+(.[0-9]+)?

<boolean_literal> ::= 
    "true" | "false"
```

## 19. JSON DSL Mapping

### 19.1 General Mapping Principles

  - **Top-Level Clauses**: Each clause maps to a corresponding JSON property
  - **Operators**: Map to enum string values as defined in the Go DSL
  - **Literals**: Map to appropriate JSON types
  - **Arrays**: Multiple items become JSON arrays
  - **Nested Structures**: Map to nested JSON objects

### 19.2 Mapping Examples

#### Filter Conditions

```
NL: AND(price <= 500, category == "electronics")
JSON: {
  "group": {
    "operator": "and",
    "conditions": [
      {"condition": {"field": "price", "operator": "lte", "value": 500}},
      {"condition": {"field": "category", "operator": "eq", "value": "electronics"}}
    ]
  }
}
```

#### Text Search

```
NL: FUZZY SEARCH "premium headphones" IN (title, description) WITH (FUZZINESS 1)
JSON: {
  "text_match": {
    "query": "premium headphones",
    "fields": ["title", "description"],
    "search_type": "fuzzy",
    "fuzziness": 1
  }
}
```

#### Computed Fields

```
NL: COMPUTE CONCAT(firstName, " ", lastName) AS fullName
JSON: {
  "computed_field_expression": {
    "type": "computed",
    "expression": {
      "function": "CONCAT",
      "arguments": ["firstName", " ", "lastName"]
    },
    "alias": "fullName"
  }
}
```

#### Joins

```
NL: JOIN LEFT orders AS customerOrders ON customerOrders.customerId == id
JSON: {
  "type": "left",
  "target_table": "orders",
  "alias": "customerOrders",
  "on": {
    "condition": {
      "field": "customerOrders.customerId",
      "operator": "eq",
      "value": "id"
    }
  }
}
```

## 20. Complete Example Query

```
WHERE 
  AND(
    status == "active",
    lastPurchaseDate > DATE_SUB(CURRENT_DATE(), "P90D"),
    lifetimeValue > 1000,
    IS_HIGH_RISK_CUSTOMER(customerId)
  )
SORT BY lifetimeValue DESC, lastName ASC
PAGINATE OFFSET 0 LIMIT 25
INCLUDE firstName, lastName, email, region, contactInfo { phone, email }
EXCLUDE password, internalNotes
COMPUTE
  CASE
    WHEN lifetimeValue > 5000 THEN "Platinum"
    WHEN lifetimeValue > 2000 THEN "Gold"
    WHEN lifetimeValue > 1000 THEN "Silver"
    ELSE "Bronze"
  END AS loyaltyStatus,
  DATE_SUB(subscriptionEndDate, "P7D") AS renewalReminder,
  GET_CUSTOMER_SEGMENT(customerId, lifetimeValue) AS segment
JOIN LEFT orders AS customerOrders ON customerOrders.customerId == id
  WHERE customerOrders.orderDate > DATE_SUB(CURRENT_DATE(), "P90D")
  INCLUDE orderId, totalAmount, items { productId, quantity }
AGGREGATE COUNT(*) AS totalOrders, SUM(customerOrders.totalAmount) AS totalSpent
GROUP BY region, loyaltyStatus
HAVING AND(COUNT(*) > 5, totalSpent > 1000)
HINT USE INDEX idx_customer_status, MAX_TIME 30
```

## 21. Error Handling and Validation

### 21.1 Syntax Errors

  - Invalid operators or keywords
  - Mismatched parentheses or brackets
  - Invalid literal formats
  - Incomplete expressions

### 21.2 Semantic Errors

  - Non-existent fields or functions
  - Type mismatches in comparisons
  - Invalid aggregation combinations
  - Circular references in joins

### 21.3 Runtime Errors

  - Function execution failures
  - Query timeout exceeded
  - Resource constraints exceeded
  - Data access permissions
