# Summary

This document contains [DuckLake's documentation](https://ducklake.select/) in a single-file easy-to-search form.
If you find any issues, please report them [as a GitHub issue](https://github.com/duckdb/ducklake-web/issues).
Contributions are very welcome in the form of [pull requests](https://github.com/duckdb/ducklake-web/pulls).
If you are considering submitting a contribution to the documentation, please consult our [contributor guide](https://github.com/duckdb/ducklake-web/blob/main/CONTRIBUTING.md).

Code repositories:

* DuckLake documentation source code: [github.com/duckdb/ducklake-web](https://github.com/duckdb/ducklake-web)
* DuckDB `ducklake` extension: [github.com/duckdb/ducklake](https://github.com/duckdb/ducklake)
# Specification {#specification}

## Introduction {#docs:stable:specification:introduction}

This page contains the specification for the DuckLake format, version 1.0.

#### Building Blocks {#docs:stable:specification:introduction::building-blocks}

DuckLake requires two main components:

* **Catalog database:** DuckLake requires a database that supports transactions and primary key constraints as defined by the [SQL-92 standard](https://en.wikipedia.org/wiki/SQL-92).
* **Data storage:** The DuckLake specification requires a storage component for storing the data in [Parquet format](https://parquet.apache.org/docs/file-format/).

##### Catalog Database {#docs:stable:specification:introduction::catalog-database}

DuckLake uses SQL tables and queries to define the catalog information (metadata, statistics, etc.).
This specification explains the schema and semantics of these:

* [Data Types](#docs:stable:specification:data_types)
* [Queries](#docs:stable:specification:queries)
* [Tables](#docs:stable:specification:tables:overview)

If you are reading this specification for the first time,
we recommend starting with the [鈥淨ueries鈥� page](#docs:stable:specification:queries),
which introduces the queries used by DuckLake.

##### Data Storage {#docs:stable:specification:introduction::data-storage}

DuckLake uses [Parquet](https://parquet.apache.org/docs/file-format/) files to represent its tables.
These files can be stored in [object storage (blob storage)](https://en.wikipedia.org/wiki/Object_storage), block storage or file storage.

## Data Types {#docs:stable:specification:data_types}

DuckLake specifies multiple different data types for field values, and also supports nested types.
The types of columns are defined in the `column_type` field of the `ducklake_column` table.

#### Primitive Types {#docs:stable:specification:data_types::primitive-types}

| Type            | Description                                                                                  |
| --------------- | -------------------------------------------------------------------------------------------- |
| `boolean`       | True or false                                                                                |
| `int8`          | 8-bit signed integer                                                                         |
| `int16`         | 16-bit signed integer                                                                        |
| `int32`         | 32-bit signed integer                                                                        |
| `int64`         | 64-bit signed integer                                                                        |
| `uint8`         | 8-bit unsigned integer                                                                       |
| `uint16`        | 16-bit unsigned integer                                                                      |
| `uint32`        | 32-bit unsigned integer                                                                      |
| `uint64`        | 64-bit unsigned integer                                                                      |
| `int128`        | 128-bit signed integer                                                                       |
| `uint128`       | 128-bit unsigned integer                                                                     |
| `float32`       | 32-bit [IEEE 754](https://en.wikipedia.org/wiki/IEEE_754) floating-point value               |
| `float64`       | 64-bit [IEEE 754](https://en.wikipedia.org/wiki/IEEE_754) floating-point value               |
| `decimal(P, S)` | Fixed-point decimal with precision `P` and scale `S`                                         |
| `time`          | Time of day, microsecond precision                                                           |
| `timetz`        | Time of day, microsecond precision, with time zone                                           |
| `date`          | Calendar date                                                                                |
| `timestamp`     | Timestamp, microsecond precision                                                             |
| `timestamptz`   | Timestamp, microsecond precision, with time zone                                             |
| `timestamp_s`   | Timestamp, second precision                                                                  |
| `timestamp_ms`  | Timestamp, millisecond precision                                                             |
| `timestamp_ns`  | Timestamp, nanosecond precision                                                              |
| `interval`      | Time interval in three different granularities: months, days, and milliseconds               |
| `varchar`       | Text                                                                                         |
| `blob`          | Binary data                                                                                  |
| `json`          | JSON                                                                                         |
| `uuid`          | [Universally unique identifier](https://en.wikipedia.org/wiki/Universally_unique_identifier) |

#### Nested Types {#docs:stable:specification:data_types::nested-types}

DuckLake supports nested types and primitive types. Nested types are defined recursively using the `parent_column` field in the [`ducklake_column`](#docs:stable:specification:tables:ducklake_column) table. For example, a column of type `INT[]` is stored as two rows: a parent column of type `list`, and a child column of type `int32` whose `parent_column` points to the parent.

The following nested types are supported:

| Type     | Description                                   |
| -------- | --------------------------------------------- |
| `list`   | Collection of values with a single child type |
| `struct` | A tuple of typed values                       |
| `map`    | A collection of key-value pairs               |

#### Semi-Structured Types {#docs:stable:specification:data_types::semi-structured-types}

| Type      | Description                                                                                        |
| --------- | -------------------------------------------------------------------------------------------------- |
| `variant` | A dynamically typed value that can hold any primitive or nested type, stored in a binary encoding. |

The `variant` type is similar to JSON but is more strongly typed internally, supports a wider range of types (e.g., `date`, `timestamp`, `decimal`), and is stored in a binary-encoded format rather than as a string. Variants are stored in Parquet files according to the [Parquet variant encoding specification](https://github.com/apache/parquet-format/blob/master/VariantEncoding.md).

Variants can be **shredded** into their constituent primitive types when all rows share a consistent schema for a given sub-field. Shredded fields are stored and queried with the same efficiency as native primitive columns. Per-file statistics for shredded sub-fields are recorded in the [`ducklake_file_variant_stats`](#docs:stable:specification:tables:ducklake_file_variant_stats) table.

> **Note.** The `variant` type is natively supported in DuckDB. For catalog databases that do not have a native variant type (e.g., PostgreSQL, SQLite), variants cannot yet be stored as inline values in those catalogs.

#### Geometry Types {#docs:stable:specification:data_types::geometry-types}

DuckLake supports geometry types using the `geometry` type of the Parquet format. The `geometry` type can store different types of spatial representations called geometry primitives, of which DuckLake supports the following:

| Geometry primitive   | Description                                                                                     |
| -------------------- | ----------------------------------------------------------------------------------------------- |
| `point`              | A single location in coordinate space.                                                          |
| `linestring`         | A sequence of points connected by straight line segments.                                       |
| `polygon`            | A planar surface defined by one exterior boundary and zero or more interior boundaries (holes). |
| `multipoint`         | A collection of `point` geometries.                                                             |
| `multilinestring`    | A collection of `linestring` geometries.                                                        |
| `multipolygon`       | A collection of `polygon` geometries.                                                           |
| `linestring_z`       | A `linestring` geometry with an additional Z (elevation) coordinate for each point.             |
| `geometrycollection` | A heterogeneous collection of geometry primitives (e.g., points, lines, polygons, etc.).        |

#### Type Encoding for Statistics {#docs:stable:specification:data_types::type-encoding-for-statistics}

Statistics values are string-encoded, as they must be stored as `VARCHAR`, since that allows storing types that are not native to the catalog database system (e.g., PostgreSQL does not support `VARIANT` natively). Statistics are stored in both `ducklake_file_column_stats` and `ducklake_table_column_stats`.
Most types follow a straightforward encoding, however some do not. The following table describes the encoding of each type:

| Type           | Description                                                                                                                                 | Example                                |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------- |
| `boolean`      | `0` or `1`                                                                                                                                  | `1`                                    |
| `int8`         | Integer string                                                                                                                              | `42`                                   |
| `int16`        | Integer string                                                                                                                              | `1000`                                 |
| `int32`        | Integer string                                                                                                                              | `100000`                               |
| `int64`        | Integer string                                                                                                                              | `100000000`                            |
| `uint8`        | Integer string                                                                                                                              | `255`                                  |
| `uint16`       | Integer string                                                                                                                              | `65535`                                |
| `uint32`       | Integer string                                                                                                                              | `4294967295`                           |
| `uint64`       | Integer string                                                                                                                              | `18446744073709551615`                 |
| `float32`      | Numeric string with special values: `inf`, `-inf`. `NaN` values are excluded from min/max, the `contains_nan` flag indicates their presence | `3.14`                                 |
| `float64`      | Numeric string with special values: `inf`, `-inf`. `NaN` values are excluded from min/max, the `contains_nan` flag indicates their presence | `3.14159265358979`                     |
| `decimal`      | Numeric string (independent of precision and scale)                                                                                         | `12345.67`                             |
| `date`         | ISO 8601 date with special values: `infinity`, `-infinity`                                                                                  | `2024-01-15`                           |
| `time`         | ISO 8601 time with microseconds if non-zero                                                                                                 | `12:30:00.123456`                      |
| `timestamp`    | ISO 8601 timestamp with microseconds if non-zero, with special values: `infinity`, `-infinity`                                              | `2024-01-15 12:30:00.123456`           |
| `timestamptz`  | ISO 8601 timestamp with UTC offset, with special values: `infinity`, `-infinity`                                                            | `2024-01-15 12:30:00.123456+00`        |
| `timestamp_s`  | ISO 8601 timestamp (second precision) with special values: `infinity`, `-infinity`                                                          | `2024-01-15 12:30:00`                  |
| `timestamp_ms` | ISO 8601 timestamp (millisecond precision) with special values: `infinity`, `-infinity`                                                     | `2024-01-15 12:30:00.123`              |
| `timestamp_ns` | ISO 8601 timestamp (nanosecond precision) with special values: `infinity`, `-infinity`                                                      | `2024-01-15 12:30:00.123456789`        |
| `varchar`      | As-is                                                                                                                                       | `hello`                                |
| `json`         | As-is                                                                                                                                       | `{"key": "value"}`                     |
| `blob`         | Hex-encoded string of the raw bytes                                                                                                         | `68656C6C6F20776F726C64`               |
| `uuid`         | Standard UUID string                                                                                                                        | `550e8400-e29b-41d4-a716-446655440000` |

The following types do not currently have min/max statistics, as they are not supported by the underlying Parquet format:
1. `int128` 
2. `uint128` 
3. `timetz` 
4. `interval`

##### Nested Types {#docs:stable:specification:data_types::nested-types}

Nested types (` list`, `struct`, `map`) do not have min/max statistics themselves, statistics are collected recursively for their child columns instead. For example:

Given the following table:

```sql
CREATE TABLE nested_types (
    col_list INT[],
    col_struct STRUCT(a INT, b VARCHAR),
    col_map MAP(VARCHAR, INT)
);
INSERT INTO nested_types VALUES
    ([1, 2, 3], {'a': 10, 'b': 'hello'}, MAP {'x': 1}),
    ([4, 5, 6], {'a': 20, 'b': 'world'}, MAP {'y': 2});
```

The parent columns (` col_list`, `col_struct`, `col_map`) have no min/max statistics. Instead, the child columns store:

| Child column | Type      | Min     | Max     |
|--|---|---|---|
| `element`    | `int32`   | `1`     | `6`     |
| `a`          | `int32`   | `10`    | `20`    |
| `b`          | `varchar` | `hello` | `world` |
| `key`        | `varchar` | `x`     | `y`     |
| `value`      | `int32`   | `1`     | `2`     |

##### Extra Statistics {#docs:stable:specification:data_types::extra-statistics}

The `geometry` and `variant` types do not use string-encoded min/max values. Instead, their statistics are stored as JSON in the `extra_stats` column of the statistics tables.

###### Geometry {#docs:stable:specification:data_types::geometry}

Geometry statistics contain a bounding box (` bbox`) and a list of geometry primitive types present in the column (` types`). For example:

```json
{
    "bbox": {
        "xmin": 0.000000,
        "xmax": 5.000000,
        "ymin": 0.000000,
        "ymax": 5.000000,
        "zmin": null,
        "zmax": null,
        "mmin": null,
        "mmax": null
    },
    "types": ["point"]
}
```

###### Variant {#docs:stable:specification:data_types::variant}

Variant statistics contain a JSON array with one entry per shredded sub-field. Each entry records the field path, its shredded type, and min/max statistics for that sub-field. If the variant cannot be shredded (i.e., has inconsistent types across rows), `extra_stats` is `NULL`. An example of a Variant `extra_stats`:

```json
[
    {
        "field_name": "root",
        "shredded_type": "int32",
        "null_count": 0,
        "min": "42",
        "max": "99",
        "num_values": 3,
        "column_size_bytes": 37,
        "any_valid": true
    }
]
```

#### Type Encoding for Data Inlining {#docs:stable:specification:data_types::type-encoding-for-data-inlining}

When storing columns for data inlining, a table is created in the catalog to store small inserts and updates in the database. Since DuckDB supports all DuckDB types, inlining types in a DuckDB catalog is straightforward. However, other catalogs might require encodings for non-native types.

##### PostgreSQL {#docs:stable:specification:data_types::postgresql}

PostgreSQL does not natively support all DuckDB types. The following table describes how each DuckDB type is mapped to a PostgreSQL type for inlined data storage. Types not listed below (e.g., `BOOLEAN`, `SMALLINT`, `INTEGER`, `BIGINT`, `FLOAT`, `DOUBLE`, `DECIMAL`, `TIME`, `TIMETZ`, `INTERVAL`, `UUID`) are stored using their native PostgreSQL equivalents.

| DuckDB Type    | PostgreSQL Type | Encoding                                                                                                                  | Example                                                                                    |
|--|---|---|---|
| `TINYINT`      | `SMALLINT`      | Direct cast                                                                                                               | `42` -> `42`                                                                               |
| `UTINYINT`     | `INTEGER`       | Direct cast                                                                                                               | `200` -> `200`                                                                             |
| `USMALLINT`    | `INTEGER`       | Direct cast                                                                                                               | `50000` -> `50000`                                                                         |
| `UINTEGER`     | `BIGINT`        | Direct cast                                                                                                               | `3000000000` -> `3000000000`                                                               |
| `UBIGINT`      | `VARCHAR`       | Stored as text                                                                                                            | `18446744073709551615` -> `'18446744073709551615'`                                         |
| `HUGEINT`      | `VARCHAR`       | Stored as text                                                                                                            | `-170141183460469231731687303715884105728` -> `'-170141183460469231731687303715884105728'` |
| `UHUGEINT`     | `VARCHAR`       | Stored as text                                                                                                            | `340282366920938463463374607431768211455` -> `'340282366920938463463374607431768211455'`   |
| `VARCHAR`      | `BYTEA`         | Text reinterpreted as bytes. PostgreSQL cannot store null bytes in `VARCHAR`/`TEXT`                                       | `'hello world'` -> `\x68656c6c6f20776f726c64`                                              |
| `BLOB`         | `BYTEA`         | Binary data stored directly as `BYTEA`                                                                                    | `'\x00\xFF'::BLOB` -> `\x00ff`                                                             |
| `JSON`         | `BYTEA`         | Text reinterpreted as bytes. DuckDB's `JSON` type is internally `VARCHAR`, so it follows the same as `VARCHAR` to `BYTEA` | `'{"key": "value"}'` -> `\x7b226b6579223a202276616c7565227d`                               |
| `DATE`         | `VARCHAR`       | Stored as text                                                                                                            | `2024-01-15` -> `'2024-01-15'`                                                             |
| `TIMESTAMP`    | `VARCHAR`       | Stored as text                                                                                                            | `2024-01-15 12:30:00.123456` -> `'2024-01-15 12:30:00.123456'`                             |
| `TIMESTAMPTZ`  | `VARCHAR`       | Stored as text                                                                                                            | `2024-01-15 12:30:00.123456+00` -> `'2024-01-15 12:30:00.123456+00'`                       |
| `TIMESTAMP_S`  | `VARCHAR`       | Stored as text                                                                                                            | `2024-01-15 12:30:00` -> `'2024-01-15 12:30:00'`                                           |
| `TIMESTAMP_MS` | `VARCHAR`       | Stored as text                                                                                                            | `2024-01-15 12:30:00.123` -> `'2024-01-15 12:30:00.123'`                                   |
| `TIMESTAMP_NS` | `VARCHAR`       | Stored as text                                                                                                            | `2024-01-15 12:30:00.123456789` -> `'2024-01-15 12:30:00.123456789'`                       |
| Nested types   | `VARCHAR`       | Stored as text, similar to stats                                                                                          | `[1, 2, 3]` -> `'[1, 2, 3]'`                                                               |

##### SQLite {#docs:stable:specification:data_types::sqlite}

SQLite has a flexible type system with only five storage classes: `INTEGER`, `REAL`, `TEXT`, `BLOB`, and `NULL`. DuckDB's SQLite scanner maps all integer types to `BIGINT` (i.e., SQLite's `INTEGER` type) and most other types to `VARCHAR` (SQLite's `TEXT` type). Only `BLOB` is stored as `BLOB`. The following table describes the non-trivial type mappings:

| DuckDB Type                                   | SQLite Type | Encoding                                          | Example                                                                                    |
|--|---|---|---|
| `BOOLEAN`                                     | `BIGINT`    | Stored as integer (` 0`/`1`)                       | `true` -> `1`                                                                              |
| `TINYINT`, `SMALLINT`, `INTEGER`, `BIGINT`    | `BIGINT`    | All integer types use SQLite's `INTEGER` affinity | `42` -> `42`                                                                               |
| `UTINYINT`, `USMALLINT`, `UINTEGER`           | `BIGINT`    | Unsigned integers fit within `BIGINT` range       | `200` -> `200`                                                                             |
| `UBIGINT`                                     | `VARCHAR`   | Stored as text                                    | `18446744073709551615` -> `'18446744073709551615'`                                         |
| `HUGEINT`                                     | `VARCHAR`   | Stored as text                                    | `-170141183460469231731687303715884105728` -> `'-170141183460469231731687303715884105728'` |
| `UHUGEINT`                                    | `VARCHAR`   | Stored as text                                    | `340282366920938463463374607431768211455` -> `'340282366920938463463374607431768211455'`   |
| `FLOAT`                                       | `VARCHAR`   | Stored as text                                    | `3.14` -> `'3.14'`                                                                         |
| `DOUBLE`                                      | `VARCHAR`   | Stored as text                                    | `3.14159265358979` -> `'3.14159265358979'`                                                 |
| `DECIMAL`                                     | `VARCHAR`   | Stored as text                                    | `12345.67` -> `'12345.67'`                                                                 |
| `UUID`                                        | `VARCHAR`   | Stored as text                                    | `550e8400-...` -> `'550e8400-...'`                                                         |
| `DATE`, `TIME`, `TIMETZ`                      | `VARCHAR`   | Stored as text                                    | `2024-01-15` -> `'2024-01-15'`                                                             |
| `TIMESTAMP`, `TIMESTAMPTZ`                    | `VARCHAR`   | Stored as text                                    | `2024-01-15 12:30:00.123456` -> `'2024-01-15 12:30:00.123456'`                             |
| `TIMESTAMP_S`, `TIMESTAMP_MS`, `TIMESTAMP_NS` | `VARCHAR`   | Stored as text                                    | `2024-01-15 12:30:00.123` -> `'2024-01-15 12:30:00.123'`                                   |
| `INTERVAL`                                    | `VARCHAR`   | Stored as text                                    | `1 year 2 months 3 days` -> `'1 year 2 months 3 days'`                                     |
| Nested types                                  | `VARCHAR`   | Stored as text, similar to stats                  | `[1, 2, 3]` -> `'[1, 2, 3]'`                                                               |

## Queries {#docs:stable:specification:queries}

This page explains the queries issued to the DuckLake catalog database for reading and writing data.

#### Reading Data {#docs:stable:specification:queries::reading-data}

DuckLake specifies _tables_ and _update transactions_ to modify them. DuckLake is not a black box: all metadata is stored as SQL tables under the user's control. Of course, they can be queried in whichever way is best for a client. Below we describe a small working example to retrieve table data.

> The information below is to provide transparency to users and to aid developers making their own implementation of DuckLake.
> If you are using the `ducklake` DuckDB extension, you do not need to worry about these: the extension is running these operations in the background for you.

##### Get Current Snapshot {#docs:stable:specification:queries::get-current-snapshot}

Before anything else we need to find a snapshot id to be queried. There can be many snapshots in the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). A snapshot id is a continuously increasing number that identifies a snapshot. In most cases, you would query the most recent one like so:

```sql
SELECT snapshot_id
FROM ducklake_snapshot
WHERE snapshot_id =
    (SELECT max(snapshot_id) FROM ducklake_snapshot);
```

##### List Schemas {#docs:stable:specification:queries::list-schemas}

A DuckLake catalog can contain many SQL-style schemas, which each can contain many tables.
These are listed in the [`ducklake_schema` table](#docs:stable:specification:tables:ducklake_schema).
Here's how we get the list of valid schemas for a given snapshot:

```sql
SELECT schema_id, schema_name
FROM ducklake_schema
WHERE
    鉄╯napshot_id鉄� >= begin_snapshot AND
    (鉄╯napshot_id鉄� < end_snapshot OR end_snapshot IS NULL);
```

where

- `鉄╯napshot_id鉄ー is a `BIGINT` referring to the `snapshot_id` column in the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot).

##### List Tables {#docs:stable:specification:queries::list-tables}

We can list the tables available in a schema for a specific snapshot using the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table):

```sql
SELECT table_id, table_name
FROM ducklake_table
WHERE
    schema_id = 鉄╯chema_id鉄� AND
    鉄╯napshot_id鉄� >= begin_snapshot AND
    (鉄╯napshot_id鉄� < end_snapshot OR end_snapshot IS NULL);
```

where

- `鉄╯chema_id鉄ー is a `BIGINT` referring to the `schema_id` column in the [`ducklake_schema` table](#docs:stable:specification:tables:ducklake_schema).
- `鉄╯napshot_id鉄ー is a `BIGINT` referring to the `snapshot_id` column in the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot).

##### Show the Structure of a Table {#docs:stable:specification:queries::show-the-structure-of-a-table}

For each given table, we can list the available top-level columns using the [`ducklake_column` table](#docs:stable:specification:tables:ducklake_column):

```sql
SELECT column_id, column_name, column_type
FROM ducklake_column
WHERE
    table_id = 鉄╰able_id鉄� AND
    parent_column IS NULL AND
    鉄╯napshot_id鉄� >= begin_snapshot AND
    (鉄╯napshot_id鉄� < end_snapshot OR end_snapshot IS NULL)
ORDER BY column_order;
```

where

- `鉄╰able_id鉄ー is a `BIGINT` referring to the `table_id` column in the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `鉄╯napshot_id鉄ー is a `BIGINT` referring to the `snapshot_id` column in the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot).

> DuckLake supports nested columns 鈥� the filter for `parent_column IS NULL` only shows the top-level columns.

For the list of supported data types, please refer to the [鈥淒ata Types鈥� page](#docs:stable:specification:data_types).

##### `SELECT` {#docs:stable:specification:queries::select}

Now that we know the table structure we can query actual data from the Parquet files that store table data. We need to join the list of data files with the list of delete files (if any). There can be at most one delete file per file in a single snapshot.

```sql
SELECT data.path AS data_file_path, del.path AS delete_file_path
FROM ducklake_data_file AS data
LEFT JOIN (
    SELECT *
    FROM ducklake_delete_file
    WHERE
        鉄╯napshot_id鉄� >= begin_snapshot AND
        (鉄╯napshot_id鉄� < end_snapshot OR end_snapshot IS NULL)
    ) AS del
USING (data_file_id)
WHERE
    data.table_id = 鉄╰able_id鉄� AND
    鉄╯napshot_id鉄� >= data.begin_snapshot AND
    (鉄╯napshot_id鉄� < data.end_snapshot OR data.end_snapshot IS NULL)
ORDER BY file_order;
```

where

- `鉄╰able_id鉄ー is a `BIGINT` referring to the `table_id` column in the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `鉄╯napshot_id鉄ー is a `BIGINT` referring to the `snapshot_id` column in the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot).

Now we have a list of files. In order to reconstruct actual table rows, we need to read all rows from the `data_file_path` files and remove the rows labeled as deleted in the `delete_file_path`.

Not all files have to contain all the columns currently defined in the table, some files may also have columns that existed previously but have been removed.

> DuckLake also supports changing the schema, see [schema evolution](#docs:stable:duckdb:usage:schema_evolution).

###### Note on Paths {#docs:stable:specification:queries::note-on-paths}

In DuckLake, paths can be relative to the initially specified data path. Whether a path is relative or not to the `data_path` prefix from [`ducklake_metadata`](#docs:stable:specification:tables:ducklake_metadata), is stored in the [`ducklake_data_file`](#docs:stable:specification:tables:ducklake_data_file) and [`ducklake_delete_file`](#docs:stable:specification:tables:ducklake_delete_file) entries (` path_is_relative`).

##### `SELECT` with File Pruning {#docs:stable:specification:queries::select-with-file-pruning}

One of the main strengths of lakehouse formats is the ability to *prune* files that cannot contain data relevant to the query.
The [`ducklake_file_column_stats` table](#docs:stable:specification:tables:ducklake_file_column_stats) contains the file-level statistics.
We can use the information there to prune the list of files to be read if a filter predicate is given.

We can get a list of all files that are part of a given table like described above. We can then reduce that list to only relevant files by querying the per-file column statistics. For example, for scalar equality we can find the relevant files using the query below:

```sql
SELECT data_file_id
FROM ducklake_file_column_stats
WHERE
    table_id = 鉄╰able_id鉄� AND
    column_id = 鉄╟olumn_id鉄� AND
    (鉄╯calar鉄� >= min_value OR min_value IS NULL) AND
    (鉄╯calar鉄� <= max_value OR max_value IS NULL);
```

where

- `鉄╰able_id鉄ー is a `BIGINT` referring to the `table_id` column in the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `鉄╟olumn_id鉄ー is a `BIGINT` referring to the `column_id` column in the [`ducklake_column` table](#docs:stable:specification:tables:ducklake_column).
- `鉄╯calar鉄ー is the scalar comparison value for the pruning.

Of course, other filter predicates like 鈥済reater than鈥� will require slightly different filtering here.

> The minimum and maximum values for each column are stored as strings and need to be cast for correct range filters on numeric columns.

#### Writing Data {#docs:stable:specification:queries::writing-data}

##### Snapshot Creation {#docs:stable:specification:queries::snapshot-creation}

Any changes to data stored in DuckLake require the creation of a new snapshot. We need to:

- create a new snapshot in [`ducklake_snapshot`](#docs:stable:specification:tables:ducklake_snapshot) and
- log the changes a snapshot made in [`ducklake_snapshot_changes`](#docs:stable:specification:tables:ducklake_snapshot_changes)

```sql
INSERT INTO ducklake_snapshot (
    snapshot_id,
    snapshot_timestamp,
    schema_version,
    next_catalog_id,
    next_file_id
)
VALUES (
    鉄╯napshot_id鉄�,
    now(),
    鉄╯chema_version鉄�,
    鉄╪ext_catalog_id鉄�,
    鉄╪ext_file_id鉄�
);

INSERT INTO ducklake_snapshot_changes (
    snapshot_id,
    snapshot_changes,
    author,
    commit_message,
    commit_extra_info
)
VALUES (
    鉄╯napshot_id鉄�,
    鉄╟hanges鉄�,
    鉄╝uthor鉄�,
    鉄╟ommit_message鉄�,
    鉄╟ommit_extra_info鉄�
);
```

where

- `鉄╯napshot_id鉄ー is the new snapshot identifier. This should be `max(snapshot_id) + 1`.
- `鉄╯chema_version鉄ー is the schema version for the new snapshot. If any schema changes are made, this needs to be incremented. Otherwise the previous snapshot's `schema_version` can be re-used.
- `鉄╪ext_catalog_id鉄ー gives the next unused identifier for tables, schemas, or views. This only has to be incremented if new catalog entries are created.
- `鉄╪ext_file_id鉄ー is the same but for data or delete files.
- `鉄╟hanges鉄ー contains a list of changes performed by the snapshot. See the list of possible values in the [`ducklake_snapshot_changes` table's documentation](#docs:stable:specification:tables:ducklake_snapshot_changes).
- `鉄╝uthor鉄ー contains information about the author of the commit (optional).
- `鉄╟ommit_message鉄ー attaches a commit message to the transaction (optional).
- `鉄╟ommit_extra_info鉄ー attaches extra information to the transaction (optional).

##### `CREATE SCHEMA` {#docs:stable:specification:queries::create-schema}

A schema is a collection of tables. In order to create a new schema, we can just insert into the [`ducklake_schema` table](#docs:stable:specification:tables:ducklake_schema):

```sql
INSERT INTO ducklake_schema (
    schema_id,
    schema_uuid,
    begin_snapshot,
    end_snapshot,
    schema_name
)
VALUES (
    鉄╯chema_id鉄�,
    uuid(),
    鉄╯napshot_id鉄�,
    NULL,
    鉄╯chema_name鉄�
);
```

where

- `鉄╯chema_id鉄ー is the new schema identifier. This should be created by incrementing `next_catalog_id` from the previous snapshot.
- `鉄╯napshot_id鉄ー is the snapshot identifier of the new snapshot as described above.
- `鉄╯chema_name鉄ー is the name of the new schema.

##### `CREATE TABLE` {#docs:stable:specification:queries::create-table}

Creating a table in a schema is very similar to creating a schema. We insert into the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table):

```sql
INSERT INTO ducklake_table (
    table_id,
    table_uuid,
    begin_snapshot,
    end_snapshot,
    schema_id,
    table_name
)
VALUES (
    鉄╰able_id鉄�,
    uuid(),
    鉄╯napshot_id鉄�,
    NULL,
    鉄╯chema_id鉄�,
    鉄╰able_name鉄�
);
```

where

- `鉄╰able_id鉄ー is the new table identifier. This should be created by further incrementing `next_catalog_id` from the previous snapshot.
- `鉄╯napshot_id鉄ー is the snapshot identifier of the new snapshot as described above.
- `鉄╯chema_id鉄ー is a `BIGINT` referring to the `schema_id` column in the [`ducklake_schema` table](#docs:stable:specification:tables:ducklake_schema) table.
- `鉄╰able_name鉄ー is the name of the new table.

A table needs some columns, we can add columns to the new table by inserting into the [`ducklake_column` table](#docs:stable:specification:tables:ducklake_column) table.
For each column to be added, we run the following query:

```sql
INSERT INTO ducklake_column (
    column_id,
    begin_snapshot,
    end_snapshot,
    table_id,
    column_order,
    column_name,
    column_type,
    nulls_allowed
)
VALUES (
    鉄╟olumn_id鉄�,
    鉄╯napshot_id鉄�,
    NULL,
    鉄╰able_id鉄�,
    鉄╟olumn_order鉄�,
    鉄╟olumn_name鉄�,
    鉄╟olumn_type鉄�,
    鉄╪ulls_allowed鉄�
);
```

where

- `鉄╟olumn_id鉄ー is the new column identifier. This ID must be unique *within the table* over its entire life time.
- `鉄╯napshot_id鉄ー is the snapshot identifier of the new snapshot as described above.
- `鉄╰able_id鉄ー is a `BIGINT` referring to the `table_id` column in the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `鉄╟olumn_order鉄ー is a number that defines where the column is placed in an ordered list of columns.
- `鉄╟olumn_name鉄ー is the name of the column.
- `鉄╟olumn_type鉄ー is the data type of the column. See the [鈥淒ata Types鈥� page](#docs:stable:specification:data_types) for details.
- `鉄╪ulls_allowed鉄ー is a boolean that defines if `NULL` values can be stored in the column. Typically set to `true`.

> We skipped some complexity in this example around default values and nested types and just left those fields as `NULL`.
> See the table schema definition for additional details.

##### `DROP TABLE` {#docs:stable:specification:queries::drop-table}

Dropping a table in DuckLake requires an update to the `end_snapshot` field in all metadata entries corresponding to the dropped table ID.

```sql
UPDATE ducklake_table
SET end_snapshot = 鉄╯napshot_id鉄� WHERE table_id = 鉄╰able_id鉄� AND end_snapshot IS NULL;

UPDATE ducklake_partition_info
SET end_snapshot = 鉄╯napshot_id鉄� WHERE table_id = 鉄╰able_id鉄� AND end_snapshot IS NULL;

UPDATE ducklake_column
SET end_snapshot = 鉄╯napshot_id鉄� WHERE table_id = 鉄╰able_id鉄� AND end_snapshot IS NULL;

UPDATE ducklake_column_tag
SET end_snapshot = 鉄╯napshot_id鉄� WHERE table_id = 鉄╰able_id鉄� AND end_snapshot IS NULL;

UPDATE ducklake_data_file
SET end_snapshot = 鉄╯napshot_id鉄� WHERE table_id = 鉄╰able_id鉄� AND end_snapshot IS NULL;

UPDATE ducklake_delete_file
SET end_snapshot = 鉄╯napshot_id鉄� WHERE table_id = 鉄╰able_id鉄� AND end_snapshot IS NULL;

UPDATE ducklake_tag
SET end_snapshot = 鉄╯napshot_id鉄� WHERE object_id  = 鉄╰able_id鉄� AND end_snapshot IS NULL;
```

where

- `鉄╯napshot_id鉄ー is the snapshot identifier of the new snapshot as described above.
- `鉄╰able_id鉄ー is the identifier of the table that will be dropped.

##### `DROP SCHEMA` {#docs:stable:specification:queries::drop-schema}

Dropping a schema in DuckLake requires updating the `end_snapshot` in the `ducklake_schema` table.

```sql
UPDATE ducklake_schema
SET
    end_snapshot = 鉄╯napshot_id鉄�
WHERE
    schema_id = 鉄╯chema_id鉄� AND
    end_snapshot IS NULL;
```

where

- `鉄╯napshot_id鉄ー is the snapshot identifier of the new snapshot as described above.
- `鉄╯chema_id鉄ー is the identifier of the schema that will be dropped.

> `DROP SCHEMA` is only allowed on empty schemas. Ensure that all tables within the schema are dropped beforehand.

##### `INSERT` {#docs:stable:specification:queries::insert}

Inserting data into a DuckLake table consists of two main steps:
first, we need to write a Parquet file containing the actual row data to storage, and
second, we need to register that file in the metadata tables and update global statistics.
Let's assume the file has already been written.

```sql
INSERT INTO ducklake_data_file (
    data_file_id,
    table_id,
    begin_snapshot,
    end_snapshot,
    path,
    path_is_relative,
    file_format,
    record_count,
    file_size_bytes,
    footer_size,
    row_id_start
)
VALUES (
    鉄╠ata_file_id鉄�,
    鉄╰able_id鉄�,
    鉄╯napshot_id鉄�,
    NULL,
    鉄╬ath鉄�,
    true,
    'parquet',
    鉄╮ecord_count鉄�,
    鉄╢ile_size_bytes鉄�,
    鉄╢ooter_size鉄�,
    鉄╮ow_id_start鉄�
);
```

where

- `鉄╠ata_file_id鉄ー is the new data file identifier. This ID must be unique *within the table* over its entire life time.
- `鉄╰able_id鉄ー is a `BIGINT` referring to the `table_id` column in the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `鉄╯napshot_id鉄ー is the snapshot identifier of the new snapshot as described above.
- `鉄╬ath鉄ー is the file name relative to the DuckLake data path from the top-level metadata.
- `鉄╮ecord_count鉄ー is the number of rows in the file.
- `鉄╢ile_size_bytes鉄ー is the file size.
- `鉄╢ooter_size鉄ー is the position of the Parquet footer. This helps with efficiently reading the file.
- `鉄╮ow_id_start鉄ー is the first logical row ID from the file. This number can be read from the [`ducklake_table_stats` table](#docs:stable:specification:tables:ducklake_table_stats) via column `next_row_id`.

We have omitted some complexity around relative paths, encrypted files, partitioning and partial files in this example.
Refer to the [`ducklake_data_file` table](#docs:stable:specification:tables:ducklake_data_file) documentation for details.

> DuckLake also supports changing the schema, see [schema evolution](#docs:stable:duckdb:usage:schema_evolution).

We also have to update some statistics in the [`ducklake_table_stats` table](#docs:stable:specification:tables:ducklake_table_stats) and [`ducklake_table_column_stats` table](#docs:stable:specification:tables:ducklake_table_column_stats) tables.

```sql
UPDATE ducklake_table_stats
SET
    record_count = record_count + 鉄╮ecord_count鉄�,
    next_row_id = next_row_id + 鉄╮ecord_count鉄�,
    file_size_bytes = file_size_bytes + 鉄╢ile_size_bytes鉄�
WHERE table_id = 鉄╰able_id鉄�;

UPDATE ducklake_table_column_stats
SET
    contains_null = contains_null OR 鉄╪ull_count鉄� > 0,
    contains_nan = contains_nan,
    min_value = min(min_value, 鉄╩in_value鉄�),
    max_value = max(max_value, 鉄╩ax_value鉄�)
WHERE
    table_id = 鉄╰able_id鉄� AND
    column_id = 鉄╟olumn_id鉄�;

INSERT INTO ducklake_file_column_stats (
    data_file_id,
    table_id,
    column_id,
    value_count,
    null_count,
    min_value,
    max_value,
    contains_nan
)
VALUES (
    鉄╠ata_file_id鉄�,
    鉄╰able_id鉄�,
    鉄╟olumn_id鉄�,
    鉄╮ecord_count鉄�,
    鉄╪ull_count鉄�,
    鉄╩in_value鉄�,
    鉄╩ax_value鉄�,
    鉄╟ontains_nan鉄�;
);
```

where

- `鉄╰able_id鉄ー is a `BIGINT` referring to the `table_id` column in the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `鉄╟olumn_id鉄ー is a `BIGINT` referring to the `column_id` column in the [`ducklake_column` table](#docs:stable:specification:tables:ducklake_column).
- `鉄╠ata_file_id鉄ー is a `BIGINT` referring to the `data_file_id` column in the [`ducklake_data_file` table](#docs:stable:specification:tables:ducklake_data_file).
- `鉄╮ecord_count鉄ー is the number of values (including `NULL` and `NaN` values) in the file column.
- `鉄╪ull_count鉄ー is the number of `NULL` values in the file column.
- `鉄╩in_value鉄ー is the *minimum* value in the file column as a string.
- `鉄╩ax_value鉄ー is the *maximum* value in the file column as a string.
- `鉄╢ile_size_bytes鉄ー is the size of the new Parquet file.
- `鉄╟ontains_nan鉄ー is a flag whether the column contains any `NaN` values. This is only relevant for floating-point types.

> This example assumes there are already rows in the table. If there are none, we need to use `INSERT` instead of `UPDATE` here.
> We also skipped the `column_size_bytes` column here, it can safely be set to `NULL`.

##### `DELETE` {#docs:stable:specification:queries::delete}

Deleting data from a DuckLake table consists of two main steps:
first, we need to write a _Parquet delete file_ containing the row index to be deleted to storage, and
second, we need to register that delete file in the metadata tables.
Let's assume the file has already been written.

```sql
INSERT INTO ducklake_delete_file (
    delete_file_id,
    table_id,
    begin_snapshot,
    end_snapshot,
    data_file_id,
    path,
    path_is_relative,
    format,
    delete_count,
    file_size_bytes,
    footer_size
)
VALUES (
    鉄╠elete_file_id鉄�,
    鉄╰able_id鉄�,
    鉄╯napshot_id鉄�,
    NULL,
    鉄╠ata_file_id鉄�,
    鉄╬ath鉄�,
    true,
    'parquet',
    鉄╠elete_count鉄�,
    鉄╢ile_size_bytes鉄�,
    鉄╢ooter_size鉄�
);
```

where

- `鉄╠elete_file_id鉄ー is the identifier for the new delete file.
- `鉄╰able_id鉄ー is a `BIGINT` referring to the `table_id` column in the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `鉄╯napshot_id鉄ー is the snapshot identifier of the new snapshot as described above.
- `鉄╠ata_file_id鉄ー is the identifier of the data file from which the rows are to be deleted.
- `鉄╬ath鉄ー is the file name relative to the DuckLake data path from the top-level metadata.
- `鉄╠elete_count鉄ー is the number of deletion records in the file.
- `鉄╢ile_size_bytes鉄ー is the file size.
- `鉄╢ooter_size鉄ー is the position of the Parquet footer. This helps with efficiently reading the file.

Notes:

- We have omitted some complexity around relative paths and encrypted files in this example. Refer to the [`ducklake_delete_file` table](#docs:stable:specification:tables:ducklake_delete_file) documentation for details.

- In DuckLake, the strategy used for `DELETE` operations is **merge-on-read**. Delete files are referenced in the [`ducklake_delete_file` table](#docs:stable:specification:tables:ducklake_delete_file).

- Please note that `DELETE` operations also do not require updates to table statistics, as the statistics are maintained as upper bounds, and deletions do not violate these bounds.

##### `UPDATE` {#docs:stable:specification:queries::update}

In DuckLake, `UPDATE` operations are expressed as a combination of a `DELETE` followed by an `INSERT`. Specifically, the outdated row is marked for deletion, and the updated version of that row is inserted. As a result, the changes to the metadata tables are equivalent to performing a `DELETE` and an `INSERT` operation sequentially within the same transaction.

## Tables {#specification:tables}

### Tables {#docs:stable:specification:tables:overview}

DuckLake v1.0 uses 28 tables to store metadata and to stage data fragments for data inlining. Below we describe all those tables and their semantics.

The following figure shows the most important 11 tables defined by the DuckLake schema:

![DuckLake schema](../images/schema/ducklake-schema-v1.0-light.svg)


#### Snapshots {#docs:stable:specification:tables:overview::snapshots}

* [`ducklake_snapshot`](#docs:stable:specification:tables:ducklake_snapshot)
* [`ducklake_snapshot_changes`](#docs:stable:specification:tables:ducklake_snapshot_changes)

#### DuckLake Schema {#docs:stable:specification:tables:overview::ducklake-schema}

* [`ducklake_schema`](#docs:stable:specification:tables:ducklake_schema)
* [`ducklake_table`](#docs:stable:specification:tables:ducklake_table)
* [`ducklake_view`](#docs:stable:specification:tables:ducklake_view)
* [`ducklake_column`](#docs:stable:specification:tables:ducklake_column)

#### Macros {#docs:stable:specification:tables:overview::macros}

* [`ducklake_macro`](#docs:stable:specification:tables:ducklake_macro)
* [`ducklake_macro_impl`](#docs:stable:specification:tables:ducklake_macro_impl)
* [`ducklake_macro_parameters`](#docs:stable:specification:tables:ducklake_macro_parameters)

#### Data Files and Tables {#docs:stable:specification:tables:overview::data-files-and-tables}

* [`ducklake_data_file`](#docs:stable:specification:tables:ducklake_data_file)
* [`ducklake_delete_file`](#docs:stable:specification:tables:ducklake_delete_file)
* [`ducklake_files_scheduled_for_deletion`](#docs:stable:specification:tables:ducklake_files_scheduled_for_deletion)
* [`ducklake_inlined_data_tables`](#docs:stable:specification:tables:ducklake_inlined_data_tables)

#### Data File Mapping {#docs:stable:specification:tables:overview::data-file-mapping}

* [`ducklake_column_mapping`](#docs:stable:specification:tables:ducklake_column_mapping)
* [`ducklake_name_mapping`](#docs:stable:specification:tables:ducklake_name_mapping)

#### Statistics {#docs:stable:specification:tables:overview::statistics}

DuckLake supports statistics on the table, column and file level.

* [`ducklake_table_stats`](#docs:stable:specification:tables:ducklake_table_stats)
* [`ducklake_table_column_stats`](#docs:stable:specification:tables:ducklake_table_column_stats)
* [`ducklake_file_column_stats`](#docs:stable:specification:tables:ducklake_file_column_stats)
* [`ducklake_file_variant_stats`](#docs:stable:specification:tables:ducklake_file_variant_stats)

#### Partitioning Information {#docs:stable:specification:tables:overview::partitioning-information}

DuckLake supports defining explicit partitioning.

* [`ducklake_partition_info`](#docs:stable:specification:tables:ducklake_partition_info)
* [`ducklake_partition_column`](#docs:stable:specification:tables:ducklake_partition_column)
* [`ducklake_file_partition_value`](#docs:stable:specification:tables:ducklake_file_partition_value)

#### Sort Information {#docs:stable:specification:tables:overview::sort-information}

DuckLake supports defining a sort order for tables to improve query performance.

* [`ducklake_sort_info`](#docs:stable:specification:tables:ducklake_sort_info)
* [`ducklake_sort_expression`](#docs:stable:specification:tables:ducklake_sort_expression)

#### Auxiliary Tables {#docs:stable:specification:tables:overview::auxiliary-tables}

* [`ducklake_metadata`](#docs:stable:specification:tables:ducklake_metadata)
* [`ducklake_tag`](#docs:stable:specification:tables:ducklake_tag)
* [`ducklake_column_tag`](#docs:stable:specification:tables:ducklake_column_tag)
* [`ducklake_schema_versions`](#docs:stable:specification:tables:ducklake_schema_versions)

#### Full Schema Creation Script {#docs:stable:specification:tables:overview::full-schema-creation-script}

For reference, you can see the full SQL script to create a DuckLake metadata database:

<details markdown='1'>
<summary markdown='span'>
Click here to see the SQL script for creating the schema for DuckLake's metadata.
</summary>
```sql
CREATE TABLE ducklake_column (column_id BIGINT, begin_snapshot BIGINT, end_snapshot BIGINT, table_id BIGINT, column_order BIGINT, column_name VARCHAR, column_type VARCHAR, initial_default VARCHAR, default_value VARCHAR, nulls_allowed BOOLEAN, parent_column BIGINT, default_value_type VARCHAR, default_value_dialect VARCHAR);
CREATE TABLE ducklake_column_mapping (mapping_id BIGINT, table_id BIGINT, "type" VARCHAR);
CREATE TABLE ducklake_column_tag (table_id BIGINT, column_id BIGINT, begin_snapshot BIGINT, end_snapshot BIGINT, "key" VARCHAR, "value" VARCHAR);
CREATE TABLE ducklake_data_file (data_file_id BIGINT PRIMARY KEY, table_id BIGINT, begin_snapshot BIGINT, end_snapshot BIGINT, file_order BIGINT, path VARCHAR, path_is_relative BOOLEAN, file_format VARCHAR, record_count BIGINT, file_size_bytes BIGINT, footer_size BIGINT, row_id_start BIGINT, partition_id BIGINT, encryption_key VARCHAR, mapping_id BIGINT, partial_max BIGINT);
CREATE TABLE ducklake_delete_file (delete_file_id BIGINT PRIMARY KEY, table_id BIGINT, begin_snapshot BIGINT, end_snapshot BIGINT, data_file_id BIGINT, path VARCHAR, path_is_relative BOOLEAN, format VARCHAR, delete_count BIGINT, file_size_bytes BIGINT, footer_size BIGINT, encryption_key VARCHAR, partial_max BIGINT);
CREATE TABLE ducklake_file_column_stats (data_file_id BIGINT, table_id BIGINT, column_id BIGINT, column_size_bytes BIGINT, value_count BIGINT, null_count BIGINT, min_value VARCHAR, max_value VARCHAR, contains_nan BOOLEAN, extra_stats VARCHAR);
CREATE TABLE ducklake_file_partition_value (data_file_id BIGINT, table_id BIGINT, partition_key_index BIGINT, partition_value VARCHAR);
CREATE TABLE ducklake_file_variant_stats (data_file_id BIGINT, table_id BIGINT, column_id BIGINT, variant_path VARCHAR, shredded_type VARCHAR, column_size_bytes BIGINT, value_count BIGINT, null_count BIGINT, min_value VARCHAR, max_value VARCHAR, contains_nan BOOLEAN, extra_stats VARCHAR);
CREATE TABLE ducklake_files_scheduled_for_deletion (data_file_id BIGINT, path VARCHAR, path_is_relative BOOLEAN, schedule_start TIMESTAMP WITH TIME ZONE);
CREATE TABLE ducklake_inlined_data_tables (table_id BIGINT, table_name VARCHAR, schema_version BIGINT);
CREATE TABLE ducklake_macro (schema_id BIGINT, macro_id BIGINT, macro_name VARCHAR, begin_snapshot BIGINT, end_snapshot BIGINT);
CREATE TABLE ducklake_macro_impl (macro_id BIGINT, impl_id BIGINT, dialect VARCHAR, "sql" VARCHAR, "type" VARCHAR);
CREATE TABLE ducklake_macro_parameters (macro_id BIGINT, impl_id BIGINT, column_id BIGINT, parameter_name VARCHAR, parameter_type VARCHAR, default_value VARCHAR, default_value_type VARCHAR);
CREATE TABLE ducklake_metadata ("key" VARCHAR NOT NULL, "value" VARCHAR NOT NULL, "scope" VARCHAR, scope_id BIGINT);
CREATE TABLE ducklake_name_mapping (mapping_id BIGINT, column_id BIGINT, source_name VARCHAR, target_field_id BIGINT, parent_column BIGINT, is_partition BOOLEAN);
CREATE TABLE ducklake_partition_column (partition_id BIGINT, table_id BIGINT, partition_key_index BIGINT, column_id BIGINT, "transform" VARCHAR);
CREATE TABLE ducklake_partition_info (partition_id BIGINT, table_id BIGINT, begin_snapshot BIGINT, end_snapshot BIGINT);
CREATE TABLE ducklake_schema (schema_id BIGINT PRIMARY KEY, schema_uuid UUID, begin_snapshot BIGINT, end_snapshot BIGINT, schema_name VARCHAR, path VARCHAR, path_is_relative BOOLEAN);
CREATE TABLE ducklake_schema_versions (begin_snapshot BIGINT, schema_version BIGINT, table_id BIGINT);
CREATE TABLE ducklake_snapshot (snapshot_id BIGINT PRIMARY KEY, snapshot_time TIMESTAMP WITH TIME ZONE, schema_version BIGINT, next_catalog_id BIGINT, next_file_id BIGINT);
CREATE TABLE ducklake_snapshot_changes (snapshot_id BIGINT PRIMARY KEY, changes_made VARCHAR, author VARCHAR, commit_message VARCHAR, commit_extra_info VARCHAR);
CREATE TABLE ducklake_sort_expression (sort_id BIGINT, table_id BIGINT, sort_key_index BIGINT, expression VARCHAR, dialect VARCHAR, sort_direction VARCHAR, null_order VARCHAR);
CREATE TABLE ducklake_sort_info (sort_id BIGINT, table_id BIGINT, begin_snapshot BIGINT, end_snapshot BIGINT);
CREATE TABLE ducklake_table (table_id BIGINT, table_uuid UUID, begin_snapshot BIGINT, end_snapshot BIGINT, schema_id BIGINT, table_name VARCHAR, path VARCHAR, path_is_relative BOOLEAN);
CREATE TABLE ducklake_table_column_stats (table_id BIGINT, column_id BIGINT, contains_null BOOLEAN, contains_nan BOOLEAN, min_value VARCHAR, max_value VARCHAR, extra_stats VARCHAR);
CREATE TABLE ducklake_table_stats (table_id BIGINT, record_count BIGINT, next_row_id BIGINT, file_size_bytes BIGINT);
CREATE TABLE ducklake_tag (object_id BIGINT, begin_snapshot BIGINT, end_snapshot BIGINT, "key" VARCHAR, "value" VARCHAR);
CREATE TABLE ducklake_view (view_id BIGINT, view_uuid UUID, begin_snapshot BIGINT, end_snapshot BIGINT, schema_id BIGINT, view_name VARCHAR, dialect VARCHAR, "sql" VARCHAR, column_aliases VARCHAR);
```
</details>

### ducklake_column {#docs:stable:specification:tables:ducklake_column}

This table describes the columns that are part of a table, including their types, default values, etc.

| Column name             | Column type |     |
| ----------------------- | ----------- | --- |
| `column_id`             | `BIGINT`    |     |
| `begin_snapshot`        | `BIGINT`    |     |
| `end_snapshot`          | `BIGINT`    |     |
| `table_id`              | `BIGINT`    |     |
| `column_order`          | `BIGINT`    |     |
| `column_name`           | `VARCHAR`   |     |
| `column_type`           | `VARCHAR`   |     |
| `initial_default`       | `VARCHAR`   |     |
| `default_value`         | `VARCHAR`   |     |
| `nulls_allowed`         | `BOOLEAN`   |     |
| `parent_column`         | `BIGINT`    |     |
| `default_value_type`    | `VARCHAR`   |     |
| `default_value_dialect` | `VARCHAR`   |     |

- `column_id` is the numeric identifier of the column. If the Parquet file includes a field identifier, it corresponds to the file's [`field_id`](https://github.com/apache/parquet-format/blob/f1fd3b9171aec7a7f0106e0203caef88d17dda82/src/main/thrift/parquet.thrift#L550). This identifier should remain consistent throughout all versions of the column, until it's dropped. The `column_id` must be unique per table.
- `begin_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). This version of the column exists *starting with* this snapshot id.
- `end_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). This version of the column exists *up to but not including* this snapshot id. If `end_snapshot` is `NULL`, this version of the column is currently valid.
- `table_id` refers to a `table_id` from the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `column_order` is a number that defines the position of the column in the list of columns. It needs to be unique within a snapshot but does not have to be contiguous (gaps are ok).
- `column_name` is the name of this version of the column, e.g., `my_column`.
- `column_type` is the type of this version of the column as defined in the list of [data types](#docs:stable:specification:data_types).
- `initial_default` is the *initial* default value as the column is being created, e.g., in `ALTER TABLE`, encoded as a string. Can be `NULL`.
- `default_value` is the *operational* default value as data is being inserted and updated, e.g., in `INSERT`, encoded as a string. Can be `NULL`.
- `nulls_allowed` defines whether `NULL` values are allowed in this version of the column. Note that default values have to be set if this is set to `false`.
- `parent_column` is the `column_id` of the parent column. This is `NULL` for top-level and non-nested columns. For example, for `STRUCT` types, this would refer to the 鈥減arent鈥� `STRUCT` column.
- `default_value_type` defines the default value type. It can either be `literal` (e.g., 42) or `expression` (e.g., `random()`)
- `default_value_dialect` defines the dialect used to interpret default values, especially useful for expressions. The dialect is the name of the system that created that value (e.g., duckdb).

> Every `ALTER` of the column creates a new version of the column, which will use the same `column_id`.

### ducklake_column_mapping {#docs:stable:specification:tables:ducklake_column_mapping}

Mappings contain the information used to map Parquet fields to column ids in the absence of `field-id`s in the Parquet file.

| Column name  | Column type |     |
| ------------ | ----------- | --- |
| `mapping_id` | `BIGINT`    |     |
| `table_id`   | `BIGINT`    |     |
| `type`       | `VARCHAR`   |     |

- `mapping_id` is the numeric identifier of the mapping. `mapping_id` is incremented from `next_catalog_id` in the `ducklake_snapshot` table.
- `table_id` refers to a `table_id` from the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `type` defines what method is used to perform the mapping.

The valid `type` values are the following:

| `type`        | Description                                            |
| ------------- | ------------------------------------------------------ |
| `map_by_name` | Map the columns based on the names in the Parquet file |

### ducklake_column_tag {#docs:stable:specification:tables:ducklake_column_tag}

Columns can also have tags, those are defined in this table.

| Column name      | Column type |             |
| ---------------- | ----------- | ----------- |
| `table_id`       | `BIGINT`    |             |
| `column_id`      | `BIGINT`    |             |
| `begin_snapshot` | `BIGINT`    |             |
| `end_snapshot`   | `BIGINT`    |             |
| `key`            | `VARCHAR`   |             |
| `value`          | `VARCHAR`   |             |

- `table_id` refers to a `table_id` from the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `column_id` refers to a `column_id` from the [`ducklake_column` table](#docs:stable:specification:tables:ducklake_column).
- `begin_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The tag is valid *starting with* this snapshot id.
- `end_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The tag is valid *up to but not including* this snapshot id. If `end_snapshot` is `NULL`, the tag is currently valid.
- `key` is an arbitrary key string. The key can't be `NULL`.
- `value` is the arbitrary value string.

### ducklake_data_file {#docs:stable:specification:tables:ducklake_data_file}

Data files contain the actual row data.

| Column name         | Column type |             |
| ------------------- | ----------- | ----------- |
| `data_file_id`      | `BIGINT`    | Primary key |
| `table_id`          | `BIGINT`    |             |
| `begin_snapshot`    | `BIGINT`    |             |
| `end_snapshot`      | `BIGINT`    |             |
| `file_order`        | `BIGINT`    |             |
| `path`              | `VARCHAR`   |             |
| `path_is_relative`  | `BOOLEAN`   |             |
| `file_format`       | `VARCHAR`   |             |
| `record_count`      | `BIGINT`    |             |
| `file_size_bytes`   | `BIGINT`    |             |
| `footer_size`       | `BIGINT`    |             |
| `row_id_start`      | `BIGINT`    |             |
| `partition_id`      | `BIGINT`    |             |
| `encryption_key`    | `VARCHAR`   |             |
| `mapping_id`        | `BIGINT`    |             |
| `partial_max`       | `BIGINT`    |             |

- `data_file_id` is the numeric identifier of the file. It is a primary key. `data_file_id` is incremented from `next_file_id` in the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot).
- `table_id` refers to a `table_id` from the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `begin_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The file is part of the table *starting with* this snapshot id.
- `end_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The file is part of the table *up to but not including* this snapshot id. If `end_snapshot` is `NULL`, the file is currently part of the table.
- `file_order` is a number that defines the vertical position of the file in the table. It needs to be unique within a snapshot but does not have to be contiguous (gaps are ok).
- `path` is the file path of the data file, e.g., `my_file.parquet` for a relative path.
- `path_is_relative` whether the `path` is relative to the [`path`](#docs:stable:specification:tables:ducklake_table) of the table (true) or an absolute path (false).
- `file_format` is the storage format of the file. Currently, only `parquet` is allowed.
- `record_count` is the number of records (row) in the file.
- `file_size_bytes` is the size of the file in bytes.
- `footer_size` is the size of the file metadata footer, in the case of Parquet the Thrift data. This is an optimization that allows for faster reading of the file.
- `row_id_start` is the first logical row id in the file. (Every row has a unique row id that is maintained.)
- `partition_id` refers to a `partition_id` from the `ducklake_partition_info` table.
- `encryption_key` contains the encryption for the file if [encryption](#docs:stable:duckdb:advanced_features:encryption) is enabled.
- `mapping_id` refers to a `mapping_id` from the [`ducklake_column_mapping` table](#docs:stable:specification:tables:ducklake_column_mapping).
- `partial_max` is the maximum snapshot id stored in a partial data file. When multiple snapshots are [merged into a single file](#docs:stable:duckdb:maintenance:merge_adjacent_files), per-row snapshot ownership is tracked via the `_ducklake_internal_snapshot_id` column embedded in the Parquet file. `partial_max` records the highest snapshot id present in that merged file, so reads and time travel can determine whether snapshot filtering is necessary. It is `NULL` for files that are not shared across snapshots.

### ducklake_delete_file {#docs:stable:specification:tables:ducklake_delete_file}

Delete files contain the row ids of rows that are deleted. Each data file will have its own delete file if any deletes are present for this data file.

| Column name        | Column type |             |
| ------------------ | ----------- | ----------- |
| `delete_file_id`   | `BIGINT`    | Primary key |
| `table_id`         | `BIGINT`    |             |
| `begin_snapshot`   | `BIGINT`    |             |
| `end_snapshot`     | `BIGINT`    |             |
| `data_file_id`     | `BIGINT`    |             |
| `path`             | `VARCHAR`   |             |
| `path_is_relative` | `BOOLEAN`   |             |
| `format`           | `VARCHAR`   |             |
| `delete_count`     | `BIGINT`    |             |
| `file_size_bytes`  | `BIGINT`    |             |
| `footer_size`      | `BIGINT`    |             |
| `encryption_key`   | `VARCHAR`   |             |
| `partial_max`      | `BIGINT`    |             |

- `delete_file_id` is the numeric identifier of the delete file. It is a primary key. `delete_file_id` is incremented from `next_file_id` in the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot).
- `table_id` refers to a `table_id` from the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `begin_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The delete file is part of the table *starting with* this snapshot id.
- `end_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The delete file is part of the table *up to but not including* this snapshot id. If `end_snapshot` is `NULL`, the delete file is currently part of the table.
- `data_file_id` refers to a `data_file_id` from the `ducklake_data_file` table.
- `path` is the file name of the delete file, e.g., `my_file-deletes.parquet` for a relative path.
- `path_is_relative` whether the `path` is relative to the [`path`](#docs:stable:specification:tables:ducklake_table) of the table (true) or an absolute path (false).
- `format` is the storage format of the delete file. Supported values are `parquet` (positional delete file) and `puffin` (Iceberg V3 deletion vector, experimental).
- `delete_count` is the number of deletion records in the file.
- `file_size_bytes` is the size of the file in bytes.
- `footer_size` is the size of the file metadata footer, in the case of Parquet the Thrift data. This is an optimization that allows for faster reading of the file.
- `encryption_key` contains the encryption for the file if [encryption](#docs:stable:duckdb:advanced_features:encryption) is enabled.
- `partial_max` is the maximum snapshot id stored in a partial deletion file. When multiple deletes target the same data file across different snapshots, the deletion file is rewritten as a partial deletion file that tracks which rows were deleted in which snapshot. This column stores the highest snapshot id present in such a file. It is `NULL` for standard (non-partial) delete files.

### ducklake_file_column_stats {#docs:stable:specification:tables:ducklake_file_column_stats}

This table contains column-level statistics for a single data file.

| Column name         | Column type |             |
| ------------------- | ----------- | ----------- |
| `data_file_id`      | `BIGINT`    |             |
| `table_id`          | `BIGINT`    |             |
| `column_id`         | `BIGINT`    |             |
| `column_size_bytes` | `BIGINT`    |             |
| `value_count`       | `BIGINT`    |             |
| `null_count`        | `BIGINT`    |             |
| `min_value`         | `VARCHAR`   |             |
| `max_value`         | `VARCHAR`   |             |
| `contains_nan`      | `BOOLEAN`   |             |
| `extra_stats`       | `VARCHAR`   |             |

- `data_file_id` refers to a `data_file_id` from the `ducklake_data_file` table.
- `table_id` refers to a `table_id` from the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `column_id` refers to a `column_id` from the [`ducklake_column` table](#docs:stable:specification:tables:ducklake_column).
- `column_size_bytes` is the byte size of the column.
- `value_count` is the number of values in the column. This does not have to correspond to the number of records in the file for nested types.
- `null_count` is the number of values in the column that are `NULL`.
- `min_value` contains the minimum value for the column, encoded as a string. This does not have to be exact but has to be a lower bound. The value has to be cast to the actual type for accurate comparison, e.g., on integer types.
- `max_value` contains the maximum value for the column, encoded as a string. This does not have to be exact but has to be an upper bound. The value has to be cast to the actual type for accurate comparison, e.g., on integer types.
- `contains_nan` is a flag whether the column contains any `NaN` values. This is only relevant for floating-point types.
- `extra_stats` contains additional type-specific statistics, such as bounding box information for geometry types or global shredded-field statistics for `variant` columns encoded as JSON.

### ducklake_file_partition_value {#docs:stable:specification:tables:ducklake_file_partition_value}

This table defines which data file belongs to which partition.

| Column name           | Column type |             |
| --------------------- | ----------- | ----------- |
| `data_file_id`        | `BIGINT`    |             |
| `table_id`            | `BIGINT`    |             |
| `partition_key_index` | `BIGINT`    |             |
| `partition_value`     | `VARCHAR`   |             |

- `data_file_id` refers to a `data_file_id` from the `ducklake_data_file` table.
- `table_id` refers to a `table_id` from the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `partition_key_index` refers to a `partition_key_index` from the `ducklake_partition_column` table.
- `partition_value` is the value that all the rows in the data file have, encoded as a string.

### ducklake_file_variant_stats {#docs:stable:specification:tables:ducklake_file_variant_stats}

This table contains per-file statistics for the shredded sub-fields of `variant` columns.

| Column name         | Column type |             |
| ------------------- | ----------- | ----------- |
| `data_file_id`      | `BIGINT`    |             |
| `table_id`          | `BIGINT`    |             |
| `column_id`         | `BIGINT`    |             |
| `variant_path`      | `VARCHAR`   |             |
| `shredded_type`     | `VARCHAR`   |             |
| `column_size_bytes` | `BIGINT`    |             |
| `value_count`       | `BIGINT`    |             |
| `null_count`        | `BIGINT`    |             |
| `min_value`         | `VARCHAR`   |             |
| `max_value`         | `VARCHAR`   |             |
| `contains_nan`      | `BOOLEAN`   |             |
| `extra_stats`       | `VARCHAR`   |             |

- `data_file_id` refers to a `data_file_id` from the [`ducklake_data_file` table](#docs:stable:specification:tables:ducklake_data_file).
- `table_id` refers to a `table_id` from the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `column_id` refers to a `column_id` from the [`ducklake_column` table](#docs:stable:specification:tables:ducklake_column) and identifies the `variant` column that contains the shredded field.
- `variant_path` is the path to the shredded sub-field within the variant. Named fields are always quoted (e.g., `"l_orderkey"`), and any quote characters within a field name are escaped by doubling them. Two special path values exist:
    - `root` 鈥� the variant value itself is a primitive (i.e., not nested).
    - `element` 鈥� the statistics apply to elements of an array-typed variant.
    - Paths can be composed, e.g., `element."a"` refers to the `a` field of each element of an array.
- `shredded_type` is the DuckLake type name of the shredded field (e.g., `int64`, `date`, `decimal(15,2)`, `varchar`).
- `column_size_bytes` is the byte size of the shredded field in this file.
- `value_count` is the number of non-null values in the shredded field.
- `null_count` is the number of rows where the shredded field is `NULL` or absent.
- `min_value` contains the minimum value for the shredded field, encoded as a string.
- `max_value` contains the maximum value for the shredded field, encoded as a string.
- `contains_nan` is a flag whether the shredded field contains any `NaN` values. Only relevant for floating-point types.
- `extra_stats` is reserved for additional type-specific statistics.

A row is written to this table for every **fully shredded** sub-field of a `variant` column in a data file. A sub-field is considered fully shredded when, for every row in the file, the field is either present as a primitive of a single consistent type, absent, or `NULL`. Fields that mix types across rows are not recorded.

Global (table-wide) statistics for shredded variant fields that are consistently shredded across every file are stored in the `extra_stats` column of the [`ducklake_table_column_stats`](#docs:stable:specification:tables:ducklake_table_column_stats) table, encoded as JSON.

### ducklake_files_scheduled_for_deletion {#docs:stable:specification:tables:ducklake_files_scheduled_for_deletion}

Files that are no longer part of any snapshot are scheduled for deletion.

| Column name        | Column type                |             |
| ------------------ | -------------------------- | ----------- |
| `data_file_id`     | `BIGINT`                   |             |
| `path`             | `VARCHAR`                  |             |
| `path_is_relative` | `BOOLEAN`                  |             |
| `schedule_start`   | `TIMESTAMPTZ`              |             |

- `data_file_id` refers to a `data_file_id` from the `ducklake_data_file` table.
- `path` is the file name of the file, e.g., `my_file.parquet`. The file name is either relative to the `data_path` value in `ducklake_metadata` or absolute. If relative, the `path_is_relative` field is set to `true`.
- `path_is_relative` defines whether the path is absolute or relative, see above.
- `schedule_start` is a timestamp of when this file was scheduled for deletion.

### ducklake_inlined_data_tables {#docs:stable:specification:tables:ducklake_inlined_data_tables}

This table links DuckLake snapshots with [inlined data tables](#docs:stable:duckdb:advanced_features:data_inlining).

| Column name       | Column type |             |
| ----------------- | ----------- | ----------- |
| `table_id`        | `BIGINT`    |             |
| `table_name`      | `VARCHAR`   |             |
| `schema_version`  | `BIGINT`    |             |

- `table_id` refers to a `table_id` from the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `table_name` is a string that names the data table for inlined data.
- `schema_version` refers to a schema version in the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot).

### ducklake_macro {#docs:stable:specification:tables:ducklake_macro}

This table stores macro definitions. Each macro is associated with a schema and tracks its lifecycle through snapshots.

| Column name      | Column type |
| ---------------- | ----------- |
| `schema_id`      | `BIGINT`    |
| `macro_id`       | `BIGINT`    |
| `macro_name`     | `VARCHAR`   |
| `begin_snapshot` | `BIGINT`    |
| `end_snapshot`   | `BIGINT`    |

- `schema_id` refers to a `schema_id` from the [`ducklake_schema` table](#docs:stable:specification:tables:ducklake_schema).
- `macro_id` is the numeric identifier of the macro. `macro_id` is incremented from `next_catalog_id` in the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot).
- `macro_name` is the name of the macro, e.g., `my_macro`.
- `begin_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The macro exists *starting with* this snapshot id.
- `end_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The macro exists *up to but not including* this snapshot id. If `end_snapshot` is `NULL`, the macro is currently valid.

### ducklake_macro_impl {#docs:stable:specification:tables:ducklake_macro_impl}

This table stores macro implementations. A single macro can have multiple implementations.

| Column name | Column type |
| ----------- | ----------- |
| `macro_id`  | `BIGINT`    |
| `impl_id`   | `BIGINT`    |
| `dialect`   | `VARCHAR`   |
| `sql`       | `VARCHAR`   |
| `type`      | `VARCHAR`   |

- `macro_id` refers to a `macro_id` from the [`ducklake_macro` table](#docs:stable:specification:tables:ducklake_macro).
- `impl_id` is the numeric identifier of the implementation within the macro.
- `dialect` is the SQL dialect of the macro implementation, e.g., `duckdb`.
- `sql` is the SQL expression or query that defines the macro body.
- `type` is the type of macro: `scalar` for scalar macros or `table` for table macros.

### ducklake_macro_parameters {#docs:stable:specification:tables:ducklake_macro_parameters}

This table stores the parameters for each macro implementation.

| Column name          | Column type |
| -------------------- | ----------- |
| `macro_id`           | `BIGINT`    |
| `impl_id`            | `BIGINT`    |
| `column_id`          | `BIGINT`    |
| `parameter_name`     | `VARCHAR`   |
| `parameter_type`     | `VARCHAR`   |
| `default_value`      | `VARCHAR`   |
| `default_value_type` | `VARCHAR`   |

- `macro_id` refers to a `macro_id` from the [`ducklake_macro` table](#docs:stable:specification:tables:ducklake_macro).
- `impl_id` refers to an `impl_id` from the [`ducklake_macro_impl` table](#docs:stable:specification:tables:ducklake_macro_impl).
- `column_id` is the positional index of the parameter within the implementation.
- `parameter_name` is the name of the parameter.
- `parameter_type` is the [DuckLake type](#docs:stable:specification:data_types) of the parameter. Set to `unknown` if the parameter type is not specified.
- `default_value` is the default value of the parameter. Set to `NULL` if no default is specified.
- `default_value_type` is the [DuckLake type](#docs:stable:specification:data_types) of the default value. Set to `unknown` if no default is specified.

### ducklake_metadata {#docs:stable:specification:tables:ducklake_metadata}

The `ducklake_metadata` table contains key/value pairs with information about the specific setup of the DuckLake catalog.

| Column name | Column type |            |
| ----------- | ----------- | ---------- |
| `key`       | `VARCHAR`   | Not `NULL` |
| `value`     | `VARCHAR`   | Not `NULL` |
| `scope`     | `VARCHAR`   |            |
| `scope_id`  | `BIGINT`    |            |

- `key` is an arbitrary key string. See below for a list of pre-defined keys. The key cannot be `NULL`.
- `value` is the arbitrary value string. The `value` cannot be `NULL`.
- `scope` defines the scope of the setting.
- `scope_id` is the id of the item that the setting is scoped to (see the table below) or `NULL` for the Global scope.

| Scope  | `scope`  | Description                                                        |
| ------ | -------- | ------------------------------------------------------------------ |
| Global | `NULL`   | The scope of the setting is global for the entire catalog.         |
| Schema | `schema` | The setting is scoped to the `schema_id` referenced by `scope_id`. |
| Table  | `table`  | The setting is scoped to the `table_id` referenced by `scope_id`.  |

Currently, the following values for `key` are specified:

| Name                           | Description                                                                                                                    | Notes                                                                                                       | Scope(s)              |
|--|---|---|---|
| `version`                      | DuckLake format version.                                                                                                       |                                                                                                             | Global                |
| `created_by`                   | Tool used to write the DuckLake.                                                                                               |                                                                                                             | Global                |
| `table`                        | A string that identifies which program wrote the schema, e.g., `DuckDB v1.5.2`.                                                |                                                                                                             | Global                |
| `data_path`                    | Path to data files, e.g., `s3://mybucket/myprefix/`.                                                                           | Has to end in `/`                                                                                           | Global                |
| `encrypted`                    | Whether or not to encrypt Parquet files written to the data path.                                                              | `'true'` or `'false'`                                                                                       | Global                |
| `data_inlining_row_limit`      | Maximum amount of rows to inline in a single insert.                                                                           |                                                                                                             | Global, Schema, Table |
| `target_file_size`             | The target data file size for insertion and compaction operations.                                                             |                                                                                                             | Global, Schema, Table |
| `parquet_row_group_size_bytes` | Number of bytes per row group in Parquet files.                                                                                |                                                                                                             | Global, Schema, Table |
| `parquet_row_group_size`       | Number of rows per row group in Parquet files.                                                                                 |                                                                                                             | Global, Schema, Table |
| `parquet_compression`          | Compression algorithm for Parquet files, e.g., `zstd`.                                                                         | `uncompressed`, `snappy`, `gzip`, `zstd`, `brotli`, `lz4`, `lz4_raw`                                        | Global, Schema, Table |
| `parquet_compression_level`    | Compression level for Parquet files.                                                                                           |                                                                                                             | Global, Schema, Table |
| `parquet_version`              | Parquet format version.                                                                                                        | `1` or `2`                                                                                                  | Global, Schema, Table |
| `hive_file_pattern`            | If partitioned data should be written in a Hive-style folder structure.                                                        | `'true'` or `'false'`                                                                                       | Global, Schema, Table |
| `require_commit_message`       | If an explicit commit message is required for a snapshot commit.                                                               | `'true'` or `'false'`                                                                                       | Global                |
| `rewrite_delete_threshold`     | Minimum amount of data (0-1) that must be removed from a file before a rewrite is warranted.                                   | Value between `0` and `1`                                                                                   | Global, Schema, Table |
| `delete_older_than`            | How old unused files must be to be removed by the `ducklake_delete_orphaned_files` and `ducklake_cleanup_old_files` functions. | Duration string (e.g., `7d`, `24h`)                                                                         | Global                |
| `expire_older_than`            | How old snapshots must be, by default, to be expired by `ducklake_expire_snapshots`.                                           | Duration string (e.g., `30d`)                                                                               | Global                |
| `auto_compact`                 | Whether compaction functions run on a table. Defaults to `true`.                                                               | Used by `ducklake_flush_inlined_data`, `ducklake_merge_adjacent_files`, `ducklake_rewrite_data_files`, etc. | Global, Schema, Table |
| `per_thread_output`            | Whether to create separate output files per thread during parallel insertion.                                                  | `'true'` or `'false'`                                                                                       | Global                |

### ducklake_name_mapping {#docs:stable:specification:tables:ducklake_name_mapping}

This table contains the information used to map a name to a [`column_id`](#docs:stable:specification:tables:ducklake_column) for a given [`mapping_id`](#docs:stable:specification:tables:ducklake_column_mapping) with the `map_by_name` type.

| Column name       | Column type |             |
| ----------------- | ----------- | ----------- |
| `mapping_id`      | `BIGINT`    |             |
| `column_id`       | `BIGINT`    |             |
| `source_name`     | `VARCHAR`   |             |
| `target_field_id` | `BIGINT`    |             |
| `parent_column`   | `BIGINT`    |             |
| `is_partition`    | `BOOLEAN`   |             |

- `mapping_id` refers to a `mapping_id` from the [`ducklake_column_mapping` table](#docs:stable:specification:tables:ducklake_column_mapping).
- `column_id` refers to a `column_id` from the [`ducklake_column` table](#docs:stable:specification:tables:ducklake_column).
- `source_name` refers to the name of the field this mapping applies to.
- `target_field_id` refers to the `field-id` that a field with the `source_name` is mapped to.
- `parent_column` is the `column_id` of the parent column. This is `NULL` for top-level and non-nested columns. For example, for `STRUCT` types, this would refer to the 鈥減arent鈥� `STRUCT` column.
- `is_partition` determines whether a column is used for hive-partitioning.

### ducklake_partition_column {#docs:stable:specification:tables:ducklake_partition_column}

Partitions can refer to one or more columns, possibly with transformations such as hashing or bucketing.

| Column name           | Column type |     |
| --------------------- | ----------- | --- |
| `partition_id`        | `BIGINT`    |     |
| `table_id`            | `BIGINT`    |     |
| `partition_key_index` | `BIGINT`    |     |
| `column_id`           | `BIGINT`    |     |
| `transform`           | `VARCHAR`   |     |

- `partition_id` refers to a `partition_id` from the `ducklake_partition_info` table.
- `table_id` refers to a `table_id` from the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `partition_key_index` defines where in the partition key the column is using 0-based indexing. For example, in a partitioning by (` a`, `b`, `c`) the `partition_key_index` of `b` would be `1`.
- `column_id` refers to a `column_id` from the [`ducklake_column` table](#docs:stable:specification:tables:ducklake_column).
- `transform` defines the type of a transform that is applied to the column value, e.g., `year`.

The table of supported transforms is as follows.

| Transform   | Source type(s)                                                                    | Description                                                                                   | Result&nbsp;type |
|--|---|---|---|
| `identity`  | Any                                                                               | Source value, unmodified                                                                      | Source type      |
| `bucket(N)` | Any                                                                               | Hash the value with Murmur3 and assign to one of `N` buckets: `(murmur3_32(v) & INT_MAX) % N` | `int32`          |
| `year`      | `date`, `timestamp`, `timestamptz`, `timestamp_s`, `timestamp_ms`, `timestamp_ns` | Extract a date or timestamp year, as years from 1970                                          | `int64`          |
| `month`     | `date`, `timestamp`, `timestamptz`, `timestamp_s`, `timestamp_ms`, `timestamp_ns` | Extract a date or timestamp month, as months from 1970-01-01                                  | `int64`          |
| `day`       | `date`, `timestamp`, `timestamptz`, `timestamp_s`, `timestamp_ms`, `timestamp_ns` | Extract a date or timestamp day, as days from 1970-01-01                                      | `int64`          |
| `hour`      | `timestamp`, `timestamptz`, `timestamp_s`, `timestamp_ms`, `timestamp_ns`         | Extract a timestamp hour, as hours from 1970-01-01 00:00:00                                   | `int64`          |

### ducklake_partition_info {#docs:stable:specification:tables:ducklake_partition_info}

| Column name      | Column type |             |
| ---------------- | ----------- | ----------- |
| `partition_id`   | `BIGINT`    |             |
| `table_id`       | `BIGINT`    |             |
| `begin_snapshot` | `BIGINT`    |             |
| `end_snapshot`   | `BIGINT`    |             |

- `partition_id` is a numeric identifier for a partition. `partition_id` is incremented from `next_catalog_id` in the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot).
- `table_id` refers to a `table_id` from the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `begin_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The partition is valid *starting with* this snapshot id.
- `end_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The partition is valid *up to but not including* this snapshot id. If `end_snapshot` is `NULL`, the partition is currently valid.

### ducklake_schema {#docs:stable:specification:tables:ducklake_schema}

This table defines valid schemas.

| Column name        | Column type |             |
| ------------------ | ----------- | ----------- |
| `schema_id`        | `BIGINT`    | Primary key |
| `schema_uuid`      | `UUID`      |             |
| `begin_snapshot`   | `BIGINT`    |             |
| `end_snapshot`     | `BIGINT`    |             |
| `schema_name`      | `VARCHAR`   |             |
| `path`             | `VARCHAR`   |             |
| `path_is_relative` | `BOOLEAN`   |             |

- `schema_id` is the numeric identifier of the schema. `schema_id` is incremented from `next_catalog_id` in the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot).
- `schema_uuid` is a UUID that gives a persistent identifier for this schema. The UUID is stored here for compatibility with existing lakehouse formats.
- `begin_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The schema exists *starting with* this snapshot id.
- `end_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The schema exists *up to but not including* this snapshot id. If `end_snapshot` is `NULL`, the schema is currently valid.
- `schema_name` is the name of the schema, e.g., `my_schema`.
- `path` is the `data_path` of the schema.
- `path_is_relative` whether the `path` is relative to the [`data_path`](#docs:stable:specification:tables:ducklake_metadata) of the catalog (true) or an absolute path (false).

### ducklake_schema_versions {#docs:stable:specification:tables:ducklake_schema_versions}

This table contains the schema versions for a range of snapshots. It is necessary to compact files with different schemas.

| Column name      | Column type |     |
| ---------------- | ----------- | --- |
| `begin_snapshot` | `BIGINT`    |     |
| `schema_version` | `BIGINT`    |     |
| `table_id`       | `BIGINT`    |     |

- `begin_snapshot` refers to a `snapshot_id` in the `ducklake_snapshot` table.
- `schema_version` refers to the `schema_version` of a `ducklake_snapshot`.
- `table_id` refers to the id of the table, allowing to track the schema changes per table.

### ducklake_snapshot {#docs:stable:specification:tables:ducklake_snapshot}

This table contains the valid snapshots in a DuckLake.

| Column name       | Column type                |             |
| ----------------- | -------------------------- | ----------- |
| `snapshot_id`     | `BIGINT`                   | Primary key |
| `snapshot_time`   | `TIMESTAMPTZ`              |             |
| `schema_version`  | `BIGINT`                   |             |
| `next_catalog_id` | `BIGINT`                   |             |
| `next_file_id`    | `BIGINT`                   |             |

- `snapshot_id` is the continuously increasing numeric identifier of the snapshot. It is a primary key and is referred to by various other tables.
- `snapshot_time` is the timestamp at which the snapshot was created.
- `schema_version` is a continuously increasing number that is incremented whenever the schema is changed, e.g., by creating a table. This allows for caching of schema information if only data is changed.
- `next_catalog_id` is a continuously increasing number that describes the next identifier for schemas, tables, views, partitions, and column name mappings. This is only changed if one of those entries is created, i.e., the schema is changing.
- `next_file_id` is a continuously increasing number that contains the next id for a data or deletion file to be added. It is only changed if data is being added or deleted, i.e., not for schema changes.

### ducklake_snapshot_changes {#docs:stable:specification:tables:ducklake_snapshot_changes}

This table lists changes that happened in a snapshot for easier conflict detection.

| Column name         | Column type |             |
| ------------------- | ----------- | ----------- |
| `snapshot_id`       | `BIGINT`    | Primary key |
| `changes_made`      | `VARCHAR`   |             |
| `author`            | `VARCHAR`   |             |
| `commit_message`    | `VARCHAR`   |             |
| `commit_extra_info` | `VARCHAR`   |             |

The `ducklake_snapshot_changes` table contains a summary of changes made by a snapshot. This table is used during [Conflict Resolution](#docs:stable:duckdb:advanced_features:conflict_resolution) to quickly find out if two snapshots have conflicting changesets.

- `snapshot_id` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot).
- `changes_made` is a comma-separated list of high-level changes made by the snapshot. The values that are contained in this list have the following format:
    - `created_schema:鉄╯chema_name鉄ー 鈥� the snapshot created a schema with the given name.
    - `created_table:鉄╰able_name鉄ー 鈥� the snapshot created a table with the given name.
    - `created_view:鉄╲iew_name鉄ー 鈥� the snapshot created a view with the given name.
    - `inserted_into_table:鉄╰able_id鉄ー 鈥� the snapshot inserted data into the given table.
    - `deleted_from_table:鉄╰able_id鉄ー 鈥� the snapshot deleted data from the given table.
    - `compacted_table:鉄╰able_id鉄ー 鈥� the snapshot ran a compaction operation on the given table.
    - `dropped_schema:鉄╯chema_id鉄ー 鈥� the snapshot dropped the given schema.
    - `dropped_table:鉄╰able_id鉄ー 鈥� the snapshot dropped the given table.
    - `dropped_view:鉄╲iew_id鉄ー 鈥� the snapshot dropped the given view.
    - `altered_table:鉄╰able_id鉄ー 鈥� the snapshot altered the given table.
    - `altered_view:鉄╲iew_id鉄ー 鈥� the snapshot altered the given view.
- `author` is the author of the snapshot.
- `commit_message` is the commit message associated with the snapshot.
- `commit_extra_info` contains extra information regarding the commit.

> Names are written in quoted-format using SQL-style escapes, i.e., the name `this "table" contains quotes` is written as `"this ""table"" contains quotes"`.

### ducklake_sort_expression {#docs:stable:specification:tables:ducklake_sort_expression}

The `ducklake_sort_expression` table stores the individual sort key expressions for each sort configuration. Each row corresponds to one expression in a `SET SORTED BY` clause.

| Column name       | Column type |             |
| ----------------- | ----------- | ----------- |
| `sort_id`         | `BIGINT`    |             |
| `table_id`        | `BIGINT`    |             |
| `sort_key_index`  | `BIGINT`    |             |
| `expression`      | `VARCHAR`   |             |
| `dialect`         | `VARCHAR`   |             |
| `sort_direction`  | `VARCHAR`   |             |
| `null_order`      | `VARCHAR`   |             |

- `sort_id` refers to a `sort_id` from the [`ducklake_sort_info` table](#docs:stable:specification:tables:ducklake_sort_info).
- `table_id` refers to a `table_id` from the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `sort_key_index` defines the position of this expression within the sort key using 0-based indexing. For example, in `SET SORTED BY (a ASC, b DESC, c ASC)` the `sort_key_index` of `b` is `1`.
- `expression` is the sort expression as a string. Currently only column names are supported.
- `dialect` identifies the SQL dialect used to interpret `expression`. Currently always `duckdb`.
- `sort_direction` is either `ASC` or `DESC`.
- `null_order` is either `NULLS_FIRST` or `NULLS_LAST`.

### ducklake_sort_info {#docs:stable:specification:tables:ducklake_sort_info}

The `ducklake_sort_info` table records the version history of sort settings for tables. Each row represents one sort configuration applied to a table, with snapshot-based validity tracking.

| Column name      | Column type |             |
| ---------------- | ----------- | ----------- |
| `sort_id`        | `BIGINT`    |             |
| `table_id`       | `BIGINT`    |             |
| `begin_snapshot` | `BIGINT`    |             |
| `end_snapshot`   | `BIGINT`    |             |

- `sort_id` is a numeric identifier for a sort configuration. `sort_id` is allocated from `next_catalog_id` in the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot).
- `table_id` refers to a `table_id` from the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `begin_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The sort configuration is valid *starting with* this snapshot id.
- `end_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The sort configuration is valid *up to but not including* this snapshot id. If `end_snapshot` is `NULL`, the sort configuration is currently active.

The expressions associated with a sort configuration are stored in [`ducklake_sort_expression`](#docs:stable:specification:tables:ducklake_sort_expression).

### ducklake_table {#docs:stable:specification:tables:ducklake_table}

This table describes tables. Inception!

| Column name        | Column type |             |
| ------------------ | ----------- | ----------- |
| `table_id`         | `BIGINT`    |             |
| `table_uuid`       | `UUID`      |             |
| `begin_snapshot`   | `BIGINT`    |             |
| `end_snapshot`     | `BIGINT`    |             |
| `schema_id`        | `BIGINT`    |             |
| `table_name`       | `VARCHAR`   |             |
| `path`             | `VARCHAR`   |             |
| `path_is_relative` | `BOOLEAN`   |             |

- `table_id` is the numeric identifier of the table. `table_id` is incremented from `next_catalog_id` in the `ducklake_snapshot` table.
- `table_uuid` is a UUID that gives a persistent identifier for this table. The UUID is stored here for compatibility with existing lakehouse formats.
- `begin_snapshot` refers to a `snapshot_id` from the `ducklake_snapshot` table. The table exists *starting with* this snapshot id.
- `end_snapshot` refers to a `snapshot_id` from the `ducklake_snapshot` table. The table exists *up to but not including* this snapshot id. If `end_snapshot` is `NULL`, the table is currently valid.
- `schema_id` refers to a `schema_id` from the `ducklake_schema` table.
- `table_name` is the name of the table, e.g., `my_table`.
- `path` is the `data_path` of the table.
- `path_is_relative` whether the `path` is relative to the [`path`](#docs:stable:specification:tables:ducklake_schema) of the schema (true) or an absolute path (false).

### ducklake_table_column_stats {#docs:stable:specification:tables:ducklake_table_column_stats}

This table contains column-level statistics for an entire table.

| Column name     | Column type |             |
| --------------- | ----------- | ----------- |
| `table_id`      | `BIGINT`    |             |
| `column_id`     | `BIGINT`    |             |
| `contains_null` | `BOOLEAN`   |             |
| `contains_nan`  | `BOOLEAN`   |             |
| `min_value`     | `VARCHAR`   |             |
| `max_value`     | `VARCHAR`   |             |
| `extra_stats`   | `VARCHAR`   |             |

- `table_id` refers to a `table_id` from the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `column_id` refers to a `column_id` from the [`ducklake_column` table](#docs:stable:specification:tables:ducklake_column).
- `contains_null` is a flag whether the column contains any `NULL` values.
- `contains_nan` is a flag whether the column contains any `NaN` values. This is only relevant for floating-point types.
- `min_value` contains the minimum value for the column, encoded as a string. This does not have to be exact but has to be a lower bound. The value has to be cast to the actual type for accurate comparison, e.g., on integer types.
- `max_value` contains the maximum value for the column, encoded as a string. This does not have to be exact but has to be an upper bound. The value has to be cast to the actual type for accurate comparison, e.g., on integer types.
- `extra_stats` contains additional type-specific statistics, such as bounding box information for geometry types or global shredded-field statistics for `variant` columns encoded as JSON. Variant global stats are populated only for sub-fields that are consistently shredded across every data file; if any file contains inconsistent data for a field, global stats for that field are omitted.

### ducklake_table_stats {#docs:stable:specification:tables:ducklake_table_stats}

This table contains table-level statistics.

| Column name       | Column type |             |
| ----------------- | ----------- | ----------- |
| `table_id`        | `BIGINT`    |             |
| `record_count`    | `BIGINT`    |             |
| `next_row_id`     | `BIGINT`    |             |
| `file_size_bytes` | `BIGINT`    |             |

- `table_id` refers to a `table_id` from the [`ducklake_table` table](#docs:stable:specification:tables:ducklake_table).
- `record_count` is the total amount of rows in the table. This can be approximate.
- `next_row_id` is the row id for newly inserted rows. Used for row lineage tracking.
- `file_size_bytes` is the total file size of all data files in the table. This can be approximate.

### ducklake_tag {#docs:stable:specification:tables:ducklake_tag}

Schemas, tables, and views, etc. can have tags, those are declared in this table.

| Column name      | Column type |             |
| ---------------- | ----------- | ----------- |
| `object_id`      | `BIGINT`    |             |
| `begin_snapshot` | `BIGINT`    |             |
| `end_snapshot`   | `BIGINT`    |             |
| `key`            | `VARCHAR`   |             |
| `value`          | `VARCHAR`   |             |

- `object_id` refers to a `schema_id`, `table_id`, etc. from various tables above.
- `begin_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The tag is valid *starting with* this snapshot id.
- `end_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The tag is valid *up to but not including* this snapshot id. If `end_snapshot` is `NULL`, the tag is currently valid.
- `key` is an arbitrary key string. The key can't be `NULL`.
- `value` is the arbitrary value string.

### ducklake_view {#docs:stable:specification:tables:ducklake_view}

This table describes SQL-style `VIEW` definitions.

| Column name      | Column type |             |
| ---------------- | ----------- | ----------- |
| `view_id`        | `BIGINT`    |             |
| `view_uuid`      | `UUID`      |             |
| `begin_snapshot` | `BIGINT`    |             |
| `end_snapshot`   | `BIGINT`    |             |
| `schema_id`      | `BIGINT`    |             |
| `view_name`      | `VARCHAR`   |             |
| `dialect`        | `VARCHAR`   |             |
| `sql`            | `VARCHAR`   |             |
| `column_aliases` | `VARCHAR`   |             |

- `view_id` is the numeric identifier of the view.  `view_id` is incremented from `next_catalog_id` in the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot).
- `view_uuid` is a UUID that gives a persistent identifier for this view. The UUID is stored here for compatibility with existing lakehouse formats.
- `begin_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The view exists *starting with* this snapshot id.
- `end_snapshot` refers to a `snapshot_id` from the [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot). The view exists *up to but not including* this snapshot id. If `end_snapshot` is `NULL`, the view is currently valid.
- `schema_id` refers to a `schema_id` from the [`ducklake_schema` table](#docs:stable:specification:tables:ducklake_schema).
- `view_name` is the name of the view, e.g., `my_view`.
- `dialect` is the SQL dialect of the view definition, e.g., `duckdb`.
- `sql` is the SQL string that defines the view, e.g., `SELECT * FROM my_table`.
- `column_aliases` contains a possible rename of the view columns. Can be `NULL` if no rename is set.

# DuckDB Extension {#duckdb}

## Introduction {#docs:stable:duckdb:introduction}

In DuckDB, DuckLake is supported through the [`ducklake` extension](https://duckdb.org/docs/current/core_extensions/ducklake).

#### Installation {#docs:stable:duckdb:introduction::installation}

Install the [latest DuckDB release](https://duckdb.org/install/).

```sql
INSTALL ducklake;
```

#### Configuration {#docs:stable:duckdb:introduction::configuration}

To use DuckLake, you need to make two decisions: which [metadata catalog database you want to use](#docs:stable:duckdb:usage:choosing_a_catalog_database) and [where you want to store those files](#docs:stable:duckdb:usage:choosing_storage). In the simplest case, you use a local DuckDB file for the metadata catalog and a local folder on your computer for file storage.

#### Creating a New Database {#docs:stable:duckdb:introduction::creating-a-new-database}

DuckLake databases are created by simply starting to use them with the [`ATTACH` statement](https://duckdb.org/docs/current/sql/statements/attach#attach). In the simplest case, you can create a local, DuckDB-backed DuckLake like so:

```sql
ATTACH 'ducklake:my_ducklake.ducklake' AS my_ducklake;
USE my_ducklake;
```

This will create a file `my_ducklake.ducklake`, which is a DuckDB database with the [DuckLake schema](#docs:stable:specification:tables:overview).

We also use `USE` so we don't have to prefix all table names with `my_ducklake`. Once data is inserted, this will also create a folder `my_ducklake.ducklake.files` in the same directory, where Parquet files are stored.

If you would like to use another directory, you can specify this in the `DATA_PATH` parameter for `ATTACH`:

```sql
ATTACH 'ducklake:my_other_ducklake.ducklake' AS my_other_ducklake (DATA_PATH 'some/other/path/');
USE my_other_ducklake;
```

The path is stored in the DuckLake metadata and does not have to be specified again to attach to an existing DuckLake catalog.

> Both `DATA_PATH` and the database file path should be relative paths (e.g., `./some/path/` or `some/path/`). Moreover, for database creation the path needs to exist already, i.e., `ATTACH 'ducklake:db/my_ducklake.ducklake' AS my_ducklake;` where `db` needs to be an existing directory.

#### Attaching an Existing Database {#docs:stable:duckdb:introduction::attaching-an-existing-database}

Attaching to an existing database also uses the `ATTACH` syntax. For example, to re-connect to the example from the previous section in a new DuckDB session, we can just type:

```sql
ATTACH 'ducklake:my_ducklake.ducklake' AS my_ducklake;
USE my_ducklake;
```

If you use the DuckDB [command line client](https://duckdb.org/docs/current/clients/cli/overview), you can pass it the filename or URL of the DuckLake as an argument:

```batch
duckdb ducklake:my_ducklake.ducklake
```

#### Using DuckLake {#docs:stable:duckdb:introduction::using-ducklake}

DuckLake is used just like any other DuckDB database. You can create schemas and tables, insert data, update data, delete data, modify table schemas, etc.

Note that 鈥� similarly to other data lake and lakehouse formats 鈥� the DuckLake format does not support indexes, primary keys, foreign keys, and `UNIQUE` or `CHECK` constraints.

Don't forget to either specify the database name of the DuckLake explicitly or use `USE`. Otherwise you might inadvertently use the temporary, in-memory database.

##### Example {#docs:stable:duckdb:introduction::example}

Let's observe what happens in DuckLake when we interact with a dataset. We will use the [Netherlands train traffic dataset](https://duckdb.org/2024/05/31/analyzing-railway-traffic-in-the-netherlands) here.

We use the example DuckLake from above:

```sql
ATTACH 'ducklake:my_ducklake.ducklake' AS my_ducklake;
USE my_ducklake;
```

Let's now import the dataset into a new table:

```sql
CREATE TABLE nl_train_stations AS
    FROM 'https://blobs.duckdb.org/nl_stations.csv';
```

Now let's peek behind the curtains. The data was just read into a Parquet file, which we can also just query.

```sql
FROM glob('my_ducklake.ducklake.files/**/*');
FROM 'my_ducklake.ducklake.files/**/*.parquet' LIMIT 10;
```

But now let's change some things around. We're really unhappy with the old name of the "Amsterdam Bijlmer ArenA" station now that the stadium has been renamed to "Johan Cruijff ArenA" and everyone here loves [Johan](https://en.wikipedia.org/wiki/Johan_Cruyff). So let's change that.

```sql
UPDATE nl_train_stations
SET name_long = 'Johan Cruijff ArenA'
WHERE code = 'ASB';
```

Poof, it's changed. We can confirm:

```sql
SELECT name_long
FROM nl_train_stations
WHERE code = 'ASB';
```

In the background, more files have appeared:

```sql
FROM glob('my_ducklake.ducklake.files/**/*');
```

We now see three files. The original data file, the rows that were deleted, and the rows that were inserted. Like most systems, DuckLake models updates as deletes followed by inserts. The deletes are just a Parquet file, we can query it:

```sql
FROM 'my_ducklake.ducklake.files/**/ducklake-*-delete.parquet';
```

The file should contain a single row that marks row 29 as deleted. A new file has appeared that contains the new values for this row.

There are now three snapshots, the table creation, data insertion, and the update. We can query that using the `snapshots()` function:

```sql
FROM my_ducklake.snapshots();
```

And we can query this table at each point:

```sql
SELECT name_long
FROM nl_train_stations AT (VERSION => 1)
WHERE code = 'ASB';
```

```sql
SELECT name_long
FROM nl_train_stations AT (VERSION => 2)
WHERE code = 'ASB';
```

Time travel finally achieved!

##### Detaching from a DuckLake {#docs:stable:duckdb:introduction::detaching-from-a-ducklake}

To detach from a DuckLake, make sure that your DuckLake is not your default database, then use the [`DETACH` statement](https://duckdb.org/docs/current/sql/statements/attach#detach):

```sql
USE memory;
DETACH my_ducklake;
```

#### Using DuckLake from a Client {#docs:stable:duckdb:introduction::using-ducklake-from-a-client}

DuckLake v1.0 is supported by with [DuckDB v1.5.2+](https://duckdb.org/docs/current/clients/overview).

## Usage {#duckdb:usage}

### Connecting {#docs:stable:duckdb:usage:connecting}

To use DuckLake, you must first either connect to an existing DuckLake, or create a new DuckLake.
The `ATTACH` command can be used to select the DuckLake instance to connect to.
In the `ATTACH` command, you must specify the [catalog database](#docs:stable:duckdb:usage:choosing_a_catalog_database) and the [data storage location](#docs:stable:duckdb:usage:choosing_storage).
When attaching, a new DuckLake is automatically created if none exists in the specified catalog database.

Note that the data storage location only has to be specified when creating a new DuckLake.
When connecting to an existing DuckLake, the data storage location is loaded from the catalog database.

```sql
ATTACH 'ducklake:鉄╩etadata_storage_location鉄�' (DATA_PATH '鉄╠ata_storage_location鉄�');
```

In addition, DuckLake connection parameters can also be stored in [secrets](https://duckdb.org/docs/current/configuration/secrets_manager).

```sql
ATTACH 'ducklake:鉄╯ecret_name鉄�';
```

#### Examples {#docs:stable:duckdb:usage:connecting::examples}

Connect to DuckLake, reading the configuration from the default (unnamed) secret:

```sql
ATTACH 'ducklake:';
```

Connect to DuckLake, reading the configuration from the secret named `my_secret`:

```sql
ATTACH 'ducklake:鉄╩y_secret鉄�';
```

Use a DuckDB database `duckdb_database.ducklake` as the catalog database with the data path defaulting to `duckdb_database.ducklake.files`:

```sql
ATTACH 'ducklake:鉄╠uckdb_database.ducklake鉄�';
```

Use a DuckDB database `duckdb_database.ducklake` as the catalog database with the data path explicitly specified as the `my_files` directory:

```sql
ATTACH 'ducklake:鉄╠uckdb_database.ducklake鉄�' (DATA_PATH '鉄╩y_files/鉄�');
```

Use a PostgreSQL database as the catalog database and an S3 path as the data path:

```sql
ATTACH 'ducklake:postgres:dbname=postgres' (DATA_PATH 's3://鉄╩y-bucket/my-data/鉄�');
```

Connect to DuckLake in read-only mode:

```sql
ATTACH 'ducklake:postgres:dbname=postgres' (READ_ONLY);
```

It is also possible to override the data path for a particular connection. This will not change the value of the `DATA_PATH` stored in the DuckLake metadata, but it will override it for the current connection allowing data to be stored in a different path.

```sql
ATTACH 'ducklake:鉄╠uckdb_database.ducklake鉄�' (DATA_PATH '鉄╫ther_data_path/鉄�', OVERRIDE_DATA_PATH true);
```

> If `OVERRIDE_DATA_PATH` is used, data under the original `DATA_PATH` will not be able to be queried in the current connection. This behavior may be changed in the future to allow to query data in a catalog regardless of the current write `DATA_PATH`.

#### Parameters {#docs:stable:duckdb:usage:connecting::parameters}

The following parameters are supported for `ATTACH`:

| Name                                               | Description                                                                                                             | Default                                                                                 |
| -------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| `CREATE_IF_NOT_EXISTS`                             | Creates a new DuckLake if the specified one does not already exist                                                      | `true`                                                                                  |
| `DATA_INLINING_ROW_LIMIT`                          | The number of rows for which [data inlining](#docs:stable:duckdb:advanced_features:data_inlining) is used  | `0`                                                                                     |
| `DATA_PATH`                                        | The storage location of the data files                                                                                  | `鉄╩etadata_file鉄�.files` for DuckDB files, required otherwise |
| `ENCRYPTED`                                        | Whether or not data is stored encrypted                                                                                 | `false`                                                                                 |
| `META_鉄≒ARAMETER_NAME鉄ー | Pass `鉄≒ARAMETER_NAME鉄ー to the catalog server                                                |                                                                                         |
| `METADATA_CATALOG`                                 | The name of the attached catalog database                                                                               | `__ducklake_metadata_鉄╠ucklake_name鉄ー                        |
| `METADATA_PARAMETERS`                              | Map of parameters to pass to the catalog server                                                                         | `{}`                                                                                    |
| `METADATA_PATH`                                    | The connection string for connecting to the metadata catalog                                                            |                                                                                         |
| `METADATA_SCHEMA`                                  | The schema in the catalog server in which to store the DuckLake tables                                                  | `main`                                                                                  |
| `AUTOMATIC_MIGRATION`                              | Automatically migrates the DuckLake catalog schema if the version does not match                                        | `false`                                                                                 |
| `OVERRIDE_DATA_PATH`                               | If the path provided in `data_path` differs from the stored path and this option is set to true, the path is overridden | `true`                                                                                  |
| `SNAPSHOT_TIME`                                    | If provided, connect to DuckLake at a snapshot at a specified point in time                                             |                                                                                         |
| `SNAPSHOT_VERSION`                                 | If provided, connect to DuckLake at a specified snapshot id                                                             |                                                                                         |

In addition, any parameters that are prefixed with `META_` are passed to the catalog used to store the metadata.
The supported parameters depend on the metadata catalog that is used.
For example, `postgres` supports the `SECRET` parameter. By using the `META_SECRET` parameter we can pass this parameter to the PostgreSQL instance.

##### Secrets {#docs:stable:duckdb:usage:connecting::secrets}

Instead of configuring the connection using `ATTACH`, secrets can be created that contain all required information for setting up a connection.
Secrets support the same list of parameters as `ATTACH`, in addition to the `METADATA_PATH` and `METADATA_PARAMETERS` parameters.

| Name                  | Description                                          | Default |
| --------------------- | ---------------------------------------------------- | ------- |
| `METADATA_PATH`       | The connection string for connecting to the metadata |         |
| `METADATA_PARAMETERS` | Map of parameters to pass to the catalog server      | `{}`    |

```sql
-- Default (unnamed) secret
CREATE SECRET (
    TYPE ducklake,
    METADATA_PATH '鉄╩etadata.duckdb鉄�',
    DATA_PATH '鉄╩etadata_files/鉄�'
);

ATTACH 'ducklake:' AS my_ducklake;

-- Named secrets
CREATE SECRET 鉄╩y_secret鉄� (
    TYPE ducklake,
    METADATA_PATH '',
    DATA_PATH 's3://鉄╩y-s3-bucket鉄�/',
    METADATA_PARAMETERS MAP {'TYPE': 'postgres', 'SECRET': 'postgres_secret'}
);
ATTACH 'ducklake:鉄╩y_secret鉄�' AS my_ducklake;
```

To persist secrets, use the [`CREATE PERSISTENT SECRET` statement](https://duckdb.org/docs/current/configuration/secrets_manager#persistent-secrets).

### Choosing a Catalog Database {#docs:stable:duckdb:usage:choosing_a_catalog_database}

You may choose different _catalog databases_ for your DuckLake.
The choice depends on several factors, including whether you need to use multiple clients, which database systems are available in your organization, etc.

On the technical side, consider the following:

* If you would like to perform **local data warehousing with a single client**, use [DuckDB](#::duckdb) as the catalog database.
* If you would like to perform **local data warehousing using multiple local clients**, use [SQLite](#::sqlite) as the catalog database.
* If you would like to operate a **multi-user lakehouse** with potentially remote clients, use [PostgreSQL](#::postgresql) as the catalog database.

#### DuckDB {#docs:stable:duckdb:usage:choosing_a_catalog_database::duckdb}

DuckDB can, of course, natively connect to DuckDB database files.
So, to get started with using DuckDB _as your DuckLake catalog database,_ you only need to install the [`ducklake` extension](https://duckdb.org/docs/current/core_extensions/ducklake) and attach to your DuckLake:

```sql
INSTALL ducklake;

ATTACH 'ducklake:metadata.ducklake' AS my_ducklake;
USE my_ducklake;
```

Note that if you are using DuckDB as your catalog database, you're limited to a single client.

#### PostgreSQL {#docs:stable:duckdb:usage:choosing_a_catalog_database::postgresql}

DuckDB can interact with a PostgreSQL database using the [`postgres` extension](https://duckdb.org/docs/current/core_extensions/postgres).
Install the `ducklake` and the `postgres` extension, and attach to your DuckLake as follows:

```sql
INSTALL ducklake;
INSTALL postgres;

-- Make sure that the database `ducklake_catalog` exists in PostgreSQL
ATTACH 'ducklake:postgres:dbname=ducklake_catalog host=localhost' AS my_ducklake
    (DATA_PATH 'data_files/');
USE my_ducklake;
```

For details on how to configure the connection, see the [`postgres` extension's documentation](https://duckdb.org/docs/current/core_extensions/postgres#configuration).

The `ducklake` and `postgres` extensions require PostgreSQL 12 or newer.

#### SQLite {#docs:stable:duckdb:usage:choosing_a_catalog_database::sqlite}

DuckDB can read and write a SQLite database file using the [`sqlite` extension](https://duckdb.org/docs/current/core_extensions/sqlite).
Install the `ducklake` and the `sqlite` extension, and attach to your DuckLake as follows:

```sql
INSTALL ducklake;
INSTALL sqlite;

ATTACH 'ducklake:sqlite:metadata.sqlite' AS my_ducklake
    (DATA_PATH 'data_files/');
USE my_ducklake;
```

While SQLite doesn't allow concurrent reads and writes, its default mode is to `ATTACH` and `DETACH` for every query, together with providing a 鈥渞etry time-out鈥� for queries when a write-lock is encountered.
This allows a reasonable amount of multi-processing support (effectively hiding the single-writer model).

#### MySQL {#docs:stable:duckdb:usage:choosing_a_catalog_database::mysql}

> **Warning.** There are a number of known issues with MySQL as a catalog for DuckLake. This is due to some limitations regarding the DuckDB MySQL connector. We therefore do not recommend to use MySQL as a catalog for DuckLake.

DuckDB can interact with a MySQL database using the [`mysql` extension](https://duckdb.org/docs/current/core_extensions/mysql).
Install the `ducklake` and the `mysql` extension, and attach to your DuckLake as follows:

```sql
INSTALL ducklake;
INSTALL mysql;

-- Make sure that the database `ducklake_catalog` exists in MySQL
ATTACH 'ducklake:mysql:db=ducklake_catalog host=localhost' AS my_ducklake
    (DATA_PATH 'data_files/');
USE my_ducklake;
```

For details on how to configure the connection, see the [`mysql` extension's documentation](https://duckdb.org/docs/current/core_extensions/mysql#configuration).

Using the `ducklake` and `mysql` extensions requires MySQL 8 or newer.

### Choosing Storage {#docs:stable:duckdb:usage:choosing_storage}

DuckLake as a concept will *never* change existing files, neither by changing existing content nor by appending to existing files. This greatly reduces the consistency requirements of file systems and greatly simplifies caching.

The DuckDB `ducklake` extension can work with any file system backend that DuckDB supports. This currently includes:
- Local files and folders
- Cloud object store like
  - [AWS S3](https://duckdb.org/docs/current/core_extensions/httpfs/s3api) and compatible (e.g., [Cloudflare R2](https://www.cloudflare.com/developer-platform/products/r2/), [Hetzner Object Storage](https://www.hetzner.com/storage/object-storage/), etc.)
  - [Google Cloud Storage](https://duckdb.org/docs/current/guides/network_cloud_storage/gcs_import)
  - [Azure Blob Store](https://duckdb.org/docs/current/core_extensions/azure)
- Virtual network attached file systems
  - [NFS](https://en.wikipedia.org/wiki/Network_File_System)
  - [SMB](https://en.wikipedia.org/wiki/Server_Message_Block)
  - [FUSE](https://en.wikipedia.org/wiki/Filesystem_in_Userspace)
  - Python [fsspec file systems](https://duckdb.org/docs/current/guides/python/filesystems)
  - etc.

When choosing storage, it's important to consider the following factors
- *Access latency and data transfer throughput*, a cloud further away will be accessible to everyone but have a higher latency. local files are very fast, but not accessible to anyone else. A compromise might be a site-local storage server.
- *Scalability and cost*, an object store is quite scalable, but potentially charges for data transfer. A local server might not incur significant operating expenses, but might struggle serving thousands of clients.

It might also be interesting to use DuckLake encryption when choosing external cloud storage.

### Snapshots {#docs:stable:duckdb:usage:snapshots}

Snapshots represent commits made to DuckLake.
Every snapshot performs a set of changes that alter the state of the database.
Snapshots can create tables, insert or delete data, and alter schemas.

Changes can only be made to DuckLake using snapshots.
Every set of changes must be accompanied by a snapshot.

#### Listing Snapshots {#docs:stable:duckdb:usage:snapshots::listing-snapshots}

The set of snapshots can be queried using the `snapshots` function. This returns a list of all snapshots and their changesets.

```sql
ATTACH 'ducklake:my_ducklake.duckdb' AS my_ducklake;
CREATE TABLE my_ducklake.people (a INTEGER, b VARCHAR);
INSERT INTO my_ducklake.people VALUES (1, 'pedro');
SELECT * FROM my_ducklake.snapshots();
```



| snapshot_id | snapshot_time                 | schema_version | changes                        | author | commit_message | commit_extra_info |
| ----------: | ----------------------------- | -------------: | ------------------------------ | ------ | -------------- | ----------------- |
|           0 | 2026-04-10 12:57:02.432386+02 |              0 | {schemas_created=[main]}       | NULL   | NULL           | NULL              |
|           1 | 2026-04-10 12:57:02.439404+02 |              1 | {tables_created=[main.people]} | NULL   | NULL           | NULL              |
|           2 | 2026-04-10 12:57:02.449289+02 |              1 | {inlined_insert=[1]}           | NULL   | NULL           | NULL              |

It is also possible to retrieve the latest snapshot id directly with a function.

```sql
FROM my_ducklake.current_snapshot();
```



|   id |
| ---: |
|    2 |

The DuckLake extension also provides a function to get the latest committed snapshot for an existing open connection. This may be useful when multiple connections are updating the same target.

```sql
FROM my_ducklake.last_committed_snapshot();
```

Which would return the following for the current connection:



|   id |
| ---: |
|    2 |

But if a new connection is open, it will return:



| id   |
| ---- |
| NULL |

#### Adding a Commit Message to a Snapshot {#docs:stable:duckdb:usage:snapshots::adding-a-commit-message-to-a-snapshot}

An author and commit message can also be added in the context of a transaction. Optionally, you can also add some extra information.

```sql
ATTACH 'ducklake:my_ducklake.duckdb' AS my_ducklake;
CREATE TABLE my_ducklake.people (a INTEGER, b VARCHAR);

-- Begin Transaction
BEGIN;
INSERT INTO my_ducklake.people VALUES (1, 'pedro');
CALL my_ducklake.set_commit_message('Pedro', 'Inserting myself', extra_info => '{''foo'': 7, ''bar'': 10}');
COMMIT;
-- End transaction
```

Query the snapshots:

```sql
SELECT * FROM my_ducklake.snapshots();
```



| snapshot_id | snapshot_time                 | schema_version | changes                        | author | commit_message   | commit_extra_info     |
| ----------: | ----------------------------- | -------------: | ------------------------------ | ------ | ---------------- | --------------------- |
|           0 | 2026-04-10 12:57:40.148846+02 |              0 | {schemas_created=[main]}       | NULL   | NULL             | NULL                  |
|           1 | 2026-04-10 12:57:40.155454+02 |              1 | {tables_created=[main.people]} | NULL   | NULL             | NULL                  |
|           2 | 2026-04-10 12:57:40.168217+02 |              1 | {inlined_insert=[1]}           | Pedro  | Inserting myself | {'foo': 7, 'bar': 10} |

### Schema Evolution {#docs:stable:duckdb:usage:schema_evolution}

DuckLake supports the evolution of the schemas of tables without requiring any data files to be rewritten. The schema of a table can be changed using the `ALTER TABLE` statement. The following statements are supported:

#### Adding Columns / Fields {#docs:stable:duckdb:usage:schema_evolution::adding-columns--fields}

To add a new column of type `INTEGER`, with default value `NULL`, use:

```sql
ALTER TABLE tbl ADD COLUMN new_column INTEGER;
```

To add a new column with an explicit default value, use:

```sql
ALTER TABLE tbl ADD COLUMN new_column VARCHAR DEFAULT 'my_default';
```

Fields can be added to columns of type `struct`. The path to the `struct` column must be specified, followed by the name of the new field and the type of the new field.

```sql
-- Add a new field of type INTEGER, with default value NULL
ALTER TABLE tbl ADD COLUMN nested_column.new_field INTEGER;
```

#### Dropping Columns / Fields {#docs:stable:duckdb:usage:schema_evolution::dropping-columns--fields}

To drop the top-level column `new_column` from the table, use:

```sql
ALTER TABLE tbl DROP COLUMN new_column;
```

Fields can be dropped by specifying the full path to the field.
For example, to drop the field `new_field` from the struct column `nested_column`, use:

```sql
ALTER TABLE tbl DROP COLUMN nested_column.new_field;
```

##### Renaming Columns / Fields {#docs:stable:duckdb:usage:schema_evolution::renaming-columns--fields}

To rename the top-level column `new_column` to `new_name`, use:

```sql
ALTER TABLE tbl RENAME new_column TO new_name;
```

Fields can be renamed by specifying the full path to the field.
For example, to rename the field `new_field` within the struct column `nested_column` to `new_name`:

```sql
ALTER TABLE tbl RENAME nested_column.new_field TO new_name;
```

##### Renaming Tables {#docs:stable:duckdb:usage:schema_evolution::renaming-tables}

To rename the table `tbl` to `tbl_new_name`, use:

```sql
ALTER TABLE tbl RENAME TO tbl_new_name;
```

#### Type Promotion {#docs:stable:duckdb:usage:schema_evolution::type-promotion}

The [types](#docs:stable:specification:data_types) of columns can be changed.

To change the type of `col1` to `BIGINT`, use:

```sql
ALTER TABLE tbl ALTER col1 SET TYPE BIGINT;
```

To change the type of field `new_field` within the struct column `nested_column` to `BIGINT`:

```sql
ALTER TABLE tbl ALTER nested_column.new_field SET TYPE BIGINT;
```

Note that not all type changes are valid. Only _type promotions_ are supported.
Type promotions must be lossless. As such, valid type promotions are promoting from a narrower type (` int32`) to a wider type (` int64`).

The full set of valid type promotions is as follows:

| Source    | Target                       |
| --------- | ---------------------------- |
| `int8`    | `int16`, `int32`, `int64`    |
| `int16`   | `int32`, `int64`             |
| `int32`   | `int64`                      |
| `uint8`   | `uint16`, `uint32`, `uint64` |
| `uint16`  | `uint32`, `uint64`           |
| `uint32`  | `uint64`                     |
| `float32` | `float64`                    |

#### Field Identifiers {#docs:stable:duckdb:usage:schema_evolution::field-identifiers}

Columns are tracked using **field identifiers**. These identifiers are stored in the `column_id` field of the [`ducklake_column` table](#docs:stable:specification:tables:ducklake_column).
The identifiers are also written to each of the data files.
For Parquet files, these are written in the [`field_id`](https://github.com/apache/parquet-format/blob/f1fd3b9171aec7a7f0106e0203caef88d17dda82/src/main/thrift/parquet.thrift#L550) field.
These identifiers are used to reconstruct the data of a table for a given snapshot.

When reading the data for a table, the schema together with the correct field identifiers is read from the [`ducklake_column` table](#docs:stable:specification:tables:ducklake_column).
Data files can contain any number of columns that exist in that schema, and can also contain columns that do not exist in that schema.

- If we drop a column, previously written data files still contain the dropped column.
- If we add a column, previously written data files do not contain the new column.
- If we change the type of a column, previously written data files contain data for the column in the old type.

To reconstruct the correct table data for a given snapshot, we must perform _field id remapping_. This is done as follows:

- Data for a column is read from the column with the corresponding `field_id`. The data types might not match in case of type promotion. In this case, the values must be cast to the correct type of the column.
- Any column that has a `field_id` that exists in the data file but not in the table schema must be ignored
- Any column that has a `field_id` that does not exist in the data file must be replaced with the `initial_default` value in the [`ducklake_column` table](#docs:stable:specification:tables:ducklake_column)

### Time Travel {#docs:stable:duckdb:usage:time_travel}

In DuckLake, every [snapshot](#docs:stable:duckdb:usage:snapshots) represents a consistent state of the database.
DuckLake keeps a record of all historic snapshots and their changesets, unless [compaction](#docs:stable:duckdb:maintenance:recommended_maintenance) is triggered and historic snapshots are explicitly deleted.

Using time travel, it is possible to query the state of the database as of any recorded snapshot.
The snapshot to query can be specified either (1) using a timestamp, or (2) explicitly using a snapshot identifier.
The `snapshots` function can be used to obtain a list of valid snapshots for a given DuckLake database.

#### Examples {#docs:stable:duckdb:usage:time_travel::examples}

Query the table at a specific snapshot version.

```sql
SELECT * FROM tbl AT (VERSION => 3);
```

Query the table as it was last week.

```sql
SELECT * FROM tbl AT (TIMESTAMP => now() - INTERVAL '1 week');
```

Attach a DuckLake database at a specific snapshot version.

```sql
ATTACH 'ducklake:metadata.duckdb' (SNAPSHOT_VERSION 3);
```

Attach a DuckLake database as it was at a specific time.

```sql
ATTACH 'ducklake:metadata.duckdb' (SNAPSHOT_TIME '2025-05-26 00:00:00');
```

### Upserting {#docs:stable:duckdb:usage:upserting}

Upserting is the combination of updating and inserting. In database operations this usually means *do something to a record if it already exists* and *do something else if it doesn't*. Many databases support primary keys to assist with this behavior. This is also the case with DuckDB, which allows for the syntax [`INSERT INTO ... ON CONFLICT`](https://duckdb.org/docs/current/sql/statements/insert#on-conflict-clause).

DuckLake, on the other hand, does not support primary keys. However, the `MERGE INTO` syntax provides the same upserting functionality.

#### Syntax {#docs:stable:duckdb:usage:upserting::syntax}

```sql
MERGE INTO target_table [target_alias]
    USING source_table [source_alias]
    ON (target_table.field = source_table.field) -- USING (field)
    WHEN MATCHED THEN UPDATE [SET] | DELETE
    WHEN NOT MATCHED THEN INSERT;
```

#### Usage {#docs:stable:duckdb:usage:upserting::usage}

First, let's create a simple table.

```sql
CREATE TABLE people (id INTEGER, name VARCHAR, salary FLOAT);
INSERT INTO people VALUES (1, 'John', 92_000.0), (2, 'Anna', 100_000.0);
```

The simplest upsert would be updating or inserting a whole row.

```sql
MERGE INTO people
    USING (
        SELECT
            unnest([3, 1]) AS id,
            unnest(['Sarah', 'John']) AS name,
            unnest([95_000.0, 105_000.0]) AS salary
    ) AS upserts
    ON (upserts.id = people.id)
    WHEN MATCHED THEN UPDATE
    WHEN NOT MATCHED THEN INSERT;

FROM people;
```

|   id | name  |   salary |
| ---: | ----- | -------: |
|    2 | Anna  | 100000.0 |
|    1 | John  | 105000.0 |
|    3 | Sarah |  95000.0 |



In the previous example we are updating the whole row if `id` matches. However, it is also a common pattern to receive a change set with some keys and the changed value. This is a good use for `SET`.

```sql
MERGE INTO people
    USING (
        SELECT
            1 AS id,
            98_000.0 AS salary
    ) AS salary_updates
    ON (salary_updates.id = people.id)
    WHEN MATCHED THEN UPDATE SET salary = salary_updates.salary;

FROM people;
```

|   id | name  |   salary |
| ---: | ----- | -------: |
|    2 | Anna  | 100000.0 |
|    3 | Sarah |  95000.0 |
|    1 | John  |  98000.0 |

Another common pattern is to receive a delete set of rows, which may only contain ids of rows to be deleted.

```sql
MERGE INTO people
    USING (
        SELECT
            1 AS id
    ) AS deletes
    ON (deletes.id = people.id)
    WHEN MATCHED THEN DELETE;

FROM people;
```

|   id | name  |   salary |
| ---: | ----- | -------: |
|    2 | Anna  | 100000.0 |
|    3 | Sarah |  95000.0 |


`MERGE INTO` also supports more complex conditions, for example for a given delete set we can decide to only remove rows that contain a `salary` greater than or equal to a certain amount.

```sql
MERGE INTO people
    USING (
        SELECT
            unnest([3, 2]) AS id
    ) AS deletes
    ON (deletes.id = people.id)
    WHEN MATCHED AND people.salary >= 100_000.0 THEN DELETE;

FROM people;
```

|   id | name  |  salary |
| ---: | ----- | ------: |
|    3 | Sarah | 95000.0 |

#### Unsupported Behavior {#docs:stable:duckdb:usage:upserting::unsupported-behavior}

Multiple `UPDATE` or `DELETE` operators are not currently supported. The following query **would not work:**

```sql
MERGE INTO people
    USING (
        SELECT
            unnest([3, 1]) AS id,
            unnest(['Sarah', 'John']) AS name,
            unnest([95_000.0, 105_000.0]) AS salary
    ) AS upserts
    ON (upserts.id = people.id)
    WHEN MATCHED AND people.salary < 100_000.0 THEN UPDATE
    -- Second update or delete condition
    WHEN MATCHED AND people.salary > 100_000.0 THEN DELETE
    WHEN NOT MATCHED THEN INSERT;
```

```console
Not implemented Error:
MERGE INTO with DuckLake only supports a single UPDATE/DELETE action currently
```

### Configuration {#docs:stable:duckdb:usage:configuration}

#### `ducklake` Extension Configuration {#docs:stable:duckdb:usage:configuration::ducklake-extension-configuration}

The `ducklake` extension also allows for some configuration regarding retry mechanism for transaction conflicts.

| Name                                       | Description                                                                                               | Default |
| ------------------------------------------ | --------------------------------------------------------------------------------------------------------- | ------: |
| `ducklake_default_data_inlining_row_limit` | Default row limit for data inlining across all connections (0 disables inlining)                          |    `10` |
| `ducklake_max_retry_count`                 | The maximum amount of retry attempts for a DuckLake transaction                                           |    `10` |
| `ducklake_retry_backoff`                   | Backoff factor for exponentially increasing retry wait time                                               |   `1.5` |
| `ducklake_retry_wait_ms`                   | Time between retries in ms                                                                                |   `100` |
| `ducklake_write_deletion_vectors`          | **Experimental.** Write Iceberg V3 deletion vectors (puffin) instead of positional delete files (parquet) | `false` |

> **Warning.** Deletion vectors are an experimental feature.

To set these configuration options, use the [`SET` statement](https://duckdb.org/docs/current/sql/statements/set):

```sql
SET ducklake_default_data_inlining_row_limit = 50;
SET ducklake_max_retry_count = 100;
SET ducklake_retry_backoff = 2;
SET ducklake_retry_wait_ms = 100;
```

#### DuckLake-Specific Configuration {#docs:stable:duckdb:usage:configuration::ducklake-specific-configuration}

DuckLake supports persistent and scoped configuration operations.
These options can be set using the `set_option` function call.
The options that have been set can be queried using the `options` function.
Configuration is persisted in the [`ducklake_metadata`](#docs:stable:specification:tables:ducklake_metadata) table.

##### DuckLake-Specific Configuration Options {#docs:stable:duckdb:usage:configuration::ducklake-specific-configuration-options}

| Name                           | Description                                                                                               | Default  |
| ------------------------------ | --------------------------------------------------------------------------------------------------------- | -------- |
| `auto_compact`                 | Whether a table is included when compaction functions are called without a specific table argument        | `true`   |
| `created_by`                   | Tool used to write the DuckLake                                                                           |          |
| `data_inlining_row_limit`      | Maximum amount of rows to inline in a single insert                                                       | `10`     |
| `data_path`                    | Path to data files                                                                                        |          |
| `delete_older_than`            | How old unused files must be to be removed by cleanup functions                                           |          |
| `encrypted`                    | Whether or not to encrypt Parquet files written to the data path                                          | `false`  |
| `expire_older_than`            | How old snapshots must be to be expired by default                                                        |          |
| `hive_file_pattern`            | If partitioned data should be written in a Hive-style folder structure                                    | `true`   |
| `parquet_compression_level`    | Compression level for Parquet files                                                                       | `3`      |
| `parquet_compression`          | Compression algorithm for Parquet files (uncompressed, snappy, gzip, zstd, brotli, lz4, lz4_raw)          | `snappy` |
| `parquet_row_group_size_bytes` | Number of bytes per row group in Parquet files                                                            |          |
| `parquet_row_group_size`       | Number of rows per row group in Parquet files                                                             | `122880` |
| `parquet_version`              | Parquet format version (1 or 2)                                                                           | `1`      |
| `per_thread_output`            | Whether to create separate output files per thread during parallel insertion                              | `false`  |
| `require_commit_message`       | If an explicit commit message is required for a snapshot commit                                           | `false`  |
| `rewrite_delete_threshold`     | Minimum fraction of data removed from a file before a rewrite is warranted (0...1)                        | `0.95`   |
| `sort_on_insert`               | Whether to sort data on `INSERT` according to `SET SORTED BY`                                             | `true`   |
| `target_file_size`             | The target data file size for insertion and compaction operations                                         | `512MB`  |
| `version`                      | DuckLake format version                                                                                   |          |
| `write_deletion_vectors`       | **Experimental.** Write Iceberg V3 deletion vectors (puffin) instead of positional delete files (parquet) | `false`  |

##### Setting DuckLake-Specific Configuration Values {#docs:stable:duckdb:usage:configuration::setting-ducklake-specific-configuration-values}

Set the global Parquet compression algorithm used when writing Parquet files:

```sql
CALL my_ducklake.set_option('parquet_compression', 'zstd');
```

Set the Parquet compression algorithm used for tables in a specific schema:

```sql
CALL my_ducklake.set_option('parquet_compression', 'zstd', schema => 'my_schema');
```

Set the Parquet compression algorithm used for a specific table:

```sql
CALL my_ducklake.set_option('parquet_compression', 'zstd', table_name => 'my_table');
```

See all options for the given attached DuckLake

```sql
FROM my_ducklake.options();
```

##### Scoping {#docs:stable:duckdb:usage:configuration::scoping}

Options can be set either globally, per-schema or per-table.
The most specific scope that is set is always used for any given setting, i.e., settings are used in the following priority:

| Priority | Setting Scope |
| -------: | ------------- |
|        1 | Table         |
|        2 | Schema        |
|        3 | Global        |
|        4 | Default       |

#### DuckLake Instance Settings {#docs:stable:duckdb:usage:configuration::ducklake-instance-settings}

The `ducklake_settings` function returns metadata about a DuckLake instance: the catalog type, the extension version and the data path.

| Column              | Description                                               |
| ------------------- | --------------------------------------------------------- |
| `catalog_type`      | Metadata catalog backend (` duckdb`, `postgres`, `sqlite`) |
| `extension_version` | Version of the `ducklake` extension                       |
| `data_path`         | Path where data files are stored                          |

```sql
FROM ducklake_settings('my_ducklake');
```

Using the convenience macro on an attached DuckLake:

```sql
FROM my_ducklake.settings();
```

### Paths {#docs:stable:duckdb:usage:paths}

DuckLake manages files stored in a separate storage location.
The paths to the files are stored in the catalog server.
Paths can be either absolute or relative to their parent path.
Whether or not a path is relative is stored in the `path_is_relative` column, alongside the `path`.
By default, all paths written by DuckLake are relative paths.

| Path type   | Path location                                 | Parent path |
| ----------- | --------------------------------------------- | ----------- |
| File path   | `ducklake_data_file` / `ducklake_delete_file` | Table path  |
| Table path  | `ducklake_table`                              | Schema path |
| Schema path | `ducklake_schema`                             | Data path   |
| Data path   | `ducklake_metadata`                           |             |

#### Default Path Structure {#docs:stable:duckdb:usage:paths::default-path-structure}

The root `data_path` is specified through the [`data_path` parameter](#docs:stable:duckdb:usage:connecting) when creating a new DuckLake.
When loading an existing DuckLake, the `data_path` is loaded from the `ducklake_metadata` if not provided.

**Schemas.** When creating a schema, a schema path is set.
By default, this path is the name of the schema for alphanumeric names `鉄╯chema_name鉄�/`, or `鉄╯chema_uuid鉄�/` otherwise.
This path is set as relative to the root `data_path`.

**Tables.** When creating a table, a table path is set. By default, this path is the name of the table for alphanumeric names `鉄╰able_name鉄┾煩/`, or `鉄╰able_uuid鉄�/` otherwise.
This path is set as relative to the path of the parent schema.

**Files.** When writing a new data or delete file to the table, a new file path is generated.
For unpartitioned tables, this path is `ducklake-鉄╱uid鉄�.parquet` 鈥� relative to the table path.

**Partitioned Files.** When writing data to a partitioned table, the files are by default written to directories in the [Hive partitioning style](https://duckdb.org/docs/current/data/partitioning/hive_partitioning#hive-partitioning).
Writing data in this manner is not required as the partition values are tracked in the catalog server itself.

This results in the following path structure:

```sql
main
鈹溾攢鈹€ unpartitioned_table
鈹�   鈹斺攢鈹€ ducklake-鉄╱uid鉄�.parquet
鈹斺攢鈹€ partitioned_table
    鈹斺攢鈹€ year=2025
        鈹斺攢鈹€ ducklake-鉄╱uid鉄�.parquet
```

## Maintenance {#duckdb:maintenance}

### Recommended Maintenance {#docs:stable:duckdb:maintenance:recommended_maintenance}

#### Metadata Maintenance {#docs:stable:duckdb:maintenance:recommended_maintenance::metadata-maintenance}

Most operations performed by DuckLake happen in the catalog database.
As such, the maintenance of the metadata server are handled by the chosen catalog database.
For example, when running PostgreSQL, it is likely sufficient to occasionally run `VACUUM` in order to ensure the system stays performant.

#### Data File Maintenance {#docs:stable:duckdb:maintenance:recommended_maintenance::data-file-maintenance}

The data files that DuckLake writes to the data directory may require maintenance depending on how the insertions take place.
When snapshots write small batches of data at a time and [data inlining is not used](#docs:stable:duckdb:advanced_features:data_inlining) small Parquet files will be written to storage.
It is recommended to merge these Parquet files using the [`merge_adjacent_files`](#docs:stable:duckdb:maintenance:merge_adjacent_files) function.

DuckLake also never deletes old data files. As old data remains accessible through [time travel](#docs:stable:duckdb:usage:time_travel).
Even when a table is dropped, the data files associated with that table are not deleted.
In order to trigger a delete of these files, the snapshots that refer to that table must be [expired](#docs:stable:duckdb:maintenance:expire_snapshots) and files should be [cleaned up](#docs:stable:duckdb:maintenance:cleanup_of_files).

If you have tables that are heavily deleted, it can be the case that you have a lot of delete files that will slow read performance. In this case, we recommend you run a function to [rewrite the deleted files](#docs:stable:duckdb:maintenance:rewrite_data_files).

If you need to run all of this operations periodically, then we recommend you use the [`CHECKPOINT`](#docs:stable:duckdb:maintenance:checkpoint) statement.

### Merge Adjacent Files {#docs:stable:duckdb:maintenance:merge_adjacent_files}

Unless [data inlining is used](#docs:stable:duckdb:advanced_features:data_inlining), each insert to DuckLake writes data to a new Parquet file.
If small insertions are performed, the Parquet files that are written are small. These only hold a few rows.
For performance reasons, it is generally recommended that Parquet files are at least a few megabytes each.

DuckLake supports merging of files **without needing to expire snapshots**.
Effectively, we can merge multiple Parquet files into a single Parquet file that holds data inserted by multiple snapshots.
The resulting file is a *partial data file*: per-row snapshot ownership is tracked via a `_ducklake_internal_snapshot_id` column embedded in the Parquet file, and the highest snapshot id present in the merged file is stored in the [`partial_max`](#docs:stable:specification:tables:ducklake_data_file) column of `ducklake_data_file`.

This preserves all of the original behavior 鈥� including time travel and data change feeds 鈥� for these snapshots.
In effect, this manner of compaction is completely transparent from a user perspective.

This compaction technique can be triggered using the `merge_adjacent_files` function. For example:

```sql
CALL ducklake_merge_adjacent_files('my_ducklake');
```

Or if you want to target a specific table within a schema:

```sql
CALL ducklake_merge_adjacent_files('my_ducklake', 't', schema => 'some_schema');
```

#### Controlling Which Tables Are Compacted {#docs:stable:duckdb:maintenance:merge_adjacent_files::controlling-which-tables-are-compacted}

When `ducklake_merge_adjacent_files` is called without a table argument, it runs on all tables where the `auto_compact` option is `true` (the default). This lets you opt specific tables or schemas out of bulk compaction calls.

> **Note** `auto_compact` does not trigger compaction automatically after writes. Compaction always runs explicitly when you call a maintenance function. The option only controls *which tables are included* when the function is called without a specific table argument.

Disable compaction for a specific table:

```sql
CALL my_ducklake.set_option('auto_compact', false, table_name => 'my_table');
```

Disable compaction for an entire schema, then re-enable it for one table within that schema:

```sql
CALL my_ducklake.set_option('auto_compact', false, schema => 'my_schema');
CALL my_ducklake.set_option('auto_compact', true, schema => 'my_schema', table_name => 'important_table');
```

With the above settings, calling `ducklake_merge_adjacent_files('my_ducklake')` will compact only `my_schema.important_table` and skip all other tables in `my_schema`. Tables in other schemas are unaffected and will still be compacted by default.

#### Advanced Options {#docs:stable:duckdb:maintenance:merge_adjacent_files::advanced-options}

The `merge_adjacent_files` function supports optional parameters to filter which files are considered for compaction and control memory usage. This enables advanced compaction strategies and more granular control over the compaction process.

- **`max_compacted_files`:** Limits the maximum number of compaction operations produced in a single call, *per table*. Compacting data files can be a very memory intensive operation, so you may consider performing this operation in batches by specifying this parameter. Note that the number of actual compacted files is highly dependent on the `target_file_size` setting.
- **`min_file_size`:** Files smaller than this size (in bytes) are excluded from compaction. If not specified, all files are considered regardless of minimum size.
- **`max_file_size`:** Files at or larger than this size (in bytes) are excluded from compaction. If not specified, it defaults to `target_file_size`. Must be greater than 0.

Example with compacted files limit (applies per table when running across all tables):

```sql
CALL ducklake_merge_adjacent_files('my_ducklake', 'my_table', max_compacted_files => 10);
```

Example with size filtering:

```sql
-- Only merge files between 10KB and 100KB
CALL ducklake_merge_adjacent_files('my_ducklake', min_file_size => 10240, max_file_size => 102400);
```

##### Example: Tiered Compaction Strategy for Streaming Workloads {#docs:stable:duckdb:maintenance:merge_adjacent_files::example-tiered-compaction-strategy-for-streaming-workloads}

File size filtering enables tiered compaction strategies, which are particularly useful for real-time/streamed ingestion patterns. A tiered approach merges files in stages:

- **Tier 0 鈫� Tier 1:** Done often, merge small files (< 1MB) into ~5MB files
- **Tier 1 鈫� Tier 2:** Done occasionally, merge medium files (1MB-10MB) into ~32MB files
- **Tier 2 鈫� Tier 3:** Done rarely, merge large files (10MB-64MB) into ~128MB files

This compaction strategy provides more predictable I/O amplification and better incremental compaction for streaming workloads.

Example tiered compaction workflow:

```sql
-- Tier 0 鈫� Tier 1: merge small files
CALL ducklake_set_option('my_ducklake', 'target_file_size', '5MB');
CALL ducklake_merge_adjacent_files('my_ducklake', max_file_size => 1048576);

-- Tier 1 鈫� Tier 2: merge medium files
CALL ducklake_set_option('my_ducklake', 'target_file_size', '32MB');
CALL ducklake_merge_adjacent_files('my_ducklake', min_file_size => 1048576, max_file_size => 10485760);

-- Tier 2 鈫� Tier 3: merge large files
CALL ducklake_set_option('my_ducklake', 'target_file_size', '128MB');
CALL ducklake_merge_adjacent_files('my_ducklake', min_file_size => 10485760, max_file_size => 67108864);
```

#### Return Values {#docs:stable:duckdb:maintenance:merge_adjacent_files::return-values}

`ducklake_merge_adjacent_files` returns one row per output file created, with the following columns:

| Column            | Type      | Description                                              |
| ----------------- | --------- | -------------------------------------------------------- |
| `schema_name`     | `VARCHAR` | Name of the schema containing the table                  |
| `table_name`      | `VARCHAR` | Name of the table                                        |
| `files_processed` | `BIGINT`  | Number of input files merged into this output file       |
| `files_created`   | `BIGINT`  | Always `1` 鈥� each row represents one output file created |

Because each row corresponds to one output file, `files_created` is always `1`. To see the total number of output files produced per table, use a `GROUP BY`:

```sql
SELECT schema_name, table_name, sum(files_created) AS total_output_files
FROM ducklake_merge_adjacent_files('my_ducklake')
GROUP BY schema_name, table_name;
```

#### Sorted Compaction {#docs:stable:duckdb:maintenance:merge_adjacent_files::sorted-compaction}

If a table has a [sort order defined](#docs:stable:duckdb:advanced_features:sorted_tables), `ducklake_merge_adjacent_files` sorts the merged output by those keys before writing the resulting Parquet file. The sort order applied is the one currently active on the table at the time compaction runs 鈥� not the order that was active when the original files were written.

> Calling this function does not immediately delete the old files.
> See the [cleanup old files](#docs:stable:duckdb:maintenance:cleanup_of_files) section on how to trigger a cleanup of these files.

### Expire Snapshots {#docs:stable:duckdb:maintenance:expire_snapshots}

DuckLake in normal operation never removes any data, even when tables are dropped or data is deleted.
Due to [time travel](#docs:stable:duckdb:usage:time_travel), the removed data is still accessible.

Data can only be physically removed from DuckLake by expiring snapshots that refer to the old data.
This can be done using the `ducklake_expire_snapshots` function.

#### Usage {#docs:stable:duckdb:maintenance:expire_snapshots::usage}

The below command expires a snapshot with a specific snapshot id.

```sql
CALL ducklake_expire_snapshots('my_ducklake', versions => [2]);
```

The below command expires snapshots older than a week.

```sql
CALL ducklake_expire_snapshots('my_ducklake', older_than => now() - INTERVAL '1 week');
```

The below command performs a *dry run*, which only lists the snapshots that will be deleted, instead of actually deleting them.

```sql
CALL ducklake_expire_snapshots('my_ducklake', dry_run => true, older_than => now() - INTERVAL '1 week');
```

It is also possible to set a DuckLake option to expire snapshots that applies to the whole catalog.

```sql
CALL my_ducklake.set_option('expire_older_than', '1 month');
```

#### Cleaning Up Files {#docs:stable:duckdb:maintenance:expire_snapshots::cleaning-up-files}

Note that expiring snapshots does not immediately delete files that are no longer referenced.
See the [cleanup old files](#docs:stable:duckdb:maintenance:cleanup_of_files) section on how to trigger a cleanup of these files.

### Cleanup of Files {#docs:stable:duckdb:maintenance:cleanup_of_files}

In DuckLake you may want to delete data files that are either a part of an expired snapshot or are simply not tracked by DuckLake anymore (i.e., orphaned files).

#### Cleanup of Files from Expired Snapshots {#docs:stable:duckdb:maintenance:cleanup_of_files::cleanup-of-files-from-expired-snapshots}

When files are no longer required in DuckLake, due to e.g. [snapshots being expired](#docs:stable:duckdb:maintenance:expire_snapshots) or [files being merged](#docs:stable:duckdb:maintenance:merge_adjacent_files), they are not immediately deleted.
The reason for this is that there might still be active queries that are scanning these files.

The files are instead added to the [`ducklake_files_scheduled_for_deletion` table](#docs:stable:specification:tables:ducklake_files_scheduled_for_deletion).
These files can then be deleted at a later point.
It is generally safe to delete files that have been scheduled for deletion more than a few days ago, provided there are no read transactions that last that long.
The files can be deleted using the `ducklake_cleanup_old_files` function.

##### Usage of the `ducklake_cleanup_old_files` Function {#docs:stable:duckdb:maintenance:cleanup_of_files::usage-of-the-ducklake_cleanup_old_files-function}

The below command deletes all files scheduled for deletion.

```sql
CALL ducklake_cleanup_old_files(
    'my_ducklake',
    cleanup_all => true
);
```

The below command deletes all files that were scheduled for deletion more than a week ago.

```sql
CALL ducklake_cleanup_old_files(
    'my_ducklake',
    older_than => now() - INTERVAL '1 week'
);
```

The below command performs a *dry run*, which only lists the files that will be deleted, instead of actually deleting them.

```sql
CALL ducklake_cleanup_old_files(
    'my_ducklake',
    dry_run => true,
    older_than => now() - INTERVAL '1 week'
);
```

#### Cleanup of Orphaned Files {#docs:stable:duckdb:maintenance:cleanup_of_files::cleanup-of-orphaned-files}

Orphan files are files that are untracked by the DuckLake metadata catalog. They may appear due to, for example, a systems failure. DuckLake provides the `ducklake_delete_orphaned_files` function to delete these files.

##### Usage of the `ducklake_delete_orphaned_files` Function {#docs:stable:duckdb:maintenance:cleanup_of_files::usage-of-the-ducklake_delete_orphaned_files-function}

The below command deletes all orphaned files.

```sql
CALL ducklake_delete_orphaned_files(
    'my_ducklake',
    cleanup_all => true
);
```

The below command deletes all files that are older than a specified time.

```sql
CALL ducklake_delete_orphaned_files(
    'my_ducklake',
    older_than => now() - INTERVAL '1 week'
);
```

The below command performs a *dry run*, which only lists the files that will be deleted, instead of actually deleting them.

```sql
CALL ducklake_delete_orphaned_files(
    'my_ducklake',
    dry_run => true,
    older_than => now() - INTERVAL '1 week'
);
```

There is also a catalog-level option available.

```sql
CALL my_ducklake.set_option('delete_older_than', '1 week');
```

### Rewrite Heavily Deleted Files {#docs:stable:duckdb:maintenance:rewrite_data_files}

DuckLake uses a merge-on-read strategy when data is deleted from a table. In short, this means that DuckLake uses a delete file which contains a pointer to the deleted records on the original file. This makes deletes very efficient. However, for heavily deleted tables, reading performance will be hindered by this approach. To solve this problem, DuckLake exposes a function called `ducklake_rewrite_data_files` that rewrites files that contain an amount of deletes bigger than a given threshold to a new file that contains non-deleted records. These files can then be further compacted with a [`ducklake_merge_adjacent_files`](#docs:stable:duckdb:maintenance:merge_adjacent_files) operation. The default value for the delete threshold is 0.95.

#### Usage {#docs:stable:duckdb:maintenance:rewrite_data_files::usage}

Apply to all tables in a catalog:

```sql
CALL ducklake_rewrite_data_files('my_ducklake');
```

Apply only to a specific table:

```sql
CALL ducklake_rewrite_data_files('my_ducklake', 't');
```

Provide a specific value for the delete threshold:

```sql
CALL ducklake_rewrite_data_files('my_ducklake', 't', delete_threshold => 0.5);
```

Set a specific threshold for the whole catalog:

```sql
CALL my_ducklake.set_option('rewrite_delete_threshold', 0.5);
```

Set a specific threshold for a schema:

```sql
CALL my_ducklake.set_option('rewrite_delete_threshold', 0.5, schema => 'my_schema');
```

Set a specific threshold for a table:

```sql
CALL my_ducklake.set_option('rewrite_delete_threshold', 0.5, table_name => 'my_table');
```

Disable automatic compaction for a specific table:

```sql
CALL my_ducklake.set_option('auto_compact', false, table_name => 'my_table');
```

#### Return Values {#docs:stable:duckdb:maintenance:rewrite_data_files::return-values}

`ducklake_rewrite_data_files` returns one row per output file created, with the following columns:

| Column            | Type      | Description                                              |
| ----------------- | --------- | -------------------------------------------------------- |
| `schema_name`     | `VARCHAR` | Name of the schema containing the table                  |
| `table_name`      | `VARCHAR` | Name of the table                                        |
| `files_processed` | `BIGINT`  | Number of input files rewritten into this output file    |
| `files_created`   | `BIGINT`  | Always `1` 鈥� each row represents one output file created |

Because each row corresponds to one output file, `files_created` is always `1`. To see the total number of output files produced per table, use a `GROUP BY`:

```sql
SELECT schema_name, table_name, sum(files_created) AS total_output_files
FROM ducklake_rewrite_data_files('my_ducklake')
GROUP BY schema_name, table_name;
```

### Checkpoint {#docs:stable:duckdb:maintenance:checkpoint}

DuckLake provides the option to implement all the maintenance functions bundled in the `CHECKPOINT` statement. This statement will run in order the following DuckLake functions:

- `ducklake_flush_inlined_data`
- `ducklake_expire_snapshots`
- `ducklake_merge_adjacent_files`
- `ducklake_rewrite_data_files`
- `ducklake_cleanup_old_files`
- `ducklake_delete_orphaned_files`

#### Usage {#docs:stable:duckdb:maintenance:checkpoint::usage}

The `CHECKPOINT` statement takes the following global DuckLake options:

- `rewrite_delete_threshold`: A threshold that determines the minimum amount of data that must be removed from a file before a rewrite is warranted (0...1). Used by `ducklake_rewrite_data_files`. Can be scoped globally, per schema, or per table.
- `delete_older_than`: How old unused files must be to be removed by the `ducklake_delete_orphaned_files` and `ducklake_cleanup_old_files` cleanup functions.
- `expire_older_than`: How old snapshots must be, by default, to be expired by `ducklake_expire_snapshots`.
- `auto_compact`: Whether the compaction functions `ducklake_flush_inlined_data`, `ducklake_merge_adjacent_files`, `ducklake_rewrite_data_files` and `ducklake_delete_orphaned_files` run on a given table. Defaults to `true`. Can be scoped globally, per schema, or per table.

If these options are not provided via the `ducklake.set_option` function, `CHECKPOINT` will use the default values when applicable and will run a `CHECKPOINT` of the whole DuckLake.

```sql
CHECKPOINT;
```

## Advanced Features {#duckdb:advanced_features}

### Constraints {#docs:stable:duckdb:advanced_features:constraints}

DuckLake has limited support for constraints.
The only constraint type that is currently supported is `NOT NULL`.
It does not support `PRIMARY KEY`, `FOREIGN KEY`, `UNIQUE` or `CHECK` constraints.

#### Examples {#docs:stable:duckdb:advanced_features:constraints::examples}

Define a column as not accepting `NULL` values using the `NOT NULL` constraint.

```sql
CREATE TABLE tbl (col INTEGER NOT NULL);
```

Add a `NOT NULL` constraint to an existing column of an existing table.

```sql
ALTER TABLE tbl ALTER col SET NOT NULL;
```

Drop a `NOT NULL` constraint from a table.

```sql
ALTER TABLE tbl ALTER col DROP NOT NULL;
```

### Conflict Resolution {#docs:stable:duckdb:advanced_features:conflict_resolution}

In DuckLake, snapshot identifiers are written in a sequential order.
The first snapshot has `snapshot_id` 0, and subsequent snapshot ids are monotonically increasing such that the second snapshot has id 1, etc.
The sequential nature of snapshot identifiers is used to **detect conflicts** between snapshots. The [`ducklake_snapshot` table](#docs:stable:specification:tables:ducklake_snapshot) has a `PRIMARY KEY` constraint defined over the `snapshot_id` column.

When two connections try to write to a `ducklake` table, they will try to write a snapshot with the same identifier and one of the transactions will trigger a `PRIMARY KEY` constraint violation and fail to commit.
When such a conflict occurs 鈥� we try to resolve the conflict. In many cases, such as when both transactions are inserting data into a table, we can retry the commit without having to rewrite any actual files.
During conflict resolution, we query the [`ducklake_snapshot_changes` table](#docs:stable:specification:tables:ducklake_snapshot_changes) to figure out the high-level set of changes that other snapshots have made in the meantime.

* If there are no logical conflicts between the changes that the snapshots have made 鈥� we automatically retry the transaction in the metadata catalog without rewriting any data files.
* If there are logical conflicts, we abort the transaction and instruct the user that conflicting changes have been made.

#### Logical Conflicts {#docs:stable:duckdb:advanced_features:conflict_resolution::logical-conflicts}

Below is a list of logical conflicts based on the snapshot's changeset. Snapshots conflict when any of the following conflicts occur:

##### Schemas {#docs:stable:duckdb:advanced_features:conflict_resolution::schemas}

* Both snapshots create a schema with the same name
* Both snapshots drop the same schema
* A snapshot tries to drop a schema in which another transaction created an entry

##### Tables / Views {#docs:stable:duckdb:advanced_features:conflict_resolution::tables--views}

* Both snapshots create a table or view with the same name in the same schema
* A snapshot tries to create a table or view in a schema that was dropped by another snapshot
* Both snapshots drop the same table or view
* A snapshot tries to alter a table or view that was dropped or altered by another snapshot

##### Data {#docs:stable:duckdb:advanced_features:conflict_resolution::data}

* A snapshot tries to insert data into a table that was dropped or altered by another snapshot
* A snapshot tries to delete data from a table that was dropped, altered, deleted from or compacted by another snapshot

##### Compaction {#docs:stable:duckdb:advanced_features:conflict_resolution::compaction}

* A snapshot tries to compact a table that was deleted from by another snapshot
* A snapshot tries to compact a table that was dropped by another snapshot

### Data Change Feed {#docs:stable:duckdb:advanced_features:data_change_feed}

In addition to allowing you to query the [state of the database at any point in time](#docs:stable:duckdb:usage:time_travel),
DuckLake allows you to query the *changes that were made between any two snapshots*. This can be done using the `table_changes` function.

#### Examples {#docs:stable:duckdb:advanced_features:data_change_feed::examples}

Consider the following DuckLake instance:

```sql
ATTACH 'ducklake:changes.duckdb' AS db (DATA_PATH 'change_files/');
-- Snapshot 1
CREATE TABLE db.tbl (id INTEGER, val VARCHAR);
-- Snapshot 2
INSERT INTO db.tbl VALUES (1, 'Hello'), (2, 'DuckLake');
-- Snapshot 3
DELETE FROM db.tbl WHERE id = 1;
-- Snapshot 4
UPDATE db.tbl SET val = concat(val, val, val);
```

##### Changes Made by a Specific Snapshot {#docs:stable:duckdb:advanced_features:data_change_feed::changes-made-by-a-specific-snapshot}

```sql
FROM db.table_changes('tbl', 2, 2);
```



| snapshot_id | rowid | change_type |  id | val      |
| ----------: | ----: | ----------- | --: | -------- |
|           2 |     0 | insert      |   1 | Hello    |
|           2 |     1 | insert      |   2 | DuckLake |

##### Changes Made between Multiple Snapshots {#docs:stable:duckdb:advanced_features:data_change_feed::changes-made-between-multiple-snapshots}

```sql
FROM db.table_changes('tbl', 3, 4)
ORDER BY snapshot_id;
```



| snapshot_id | rowid | change_type      |  id | val                      |
| ----------: | ----: | ---------------- | --: | ------------------------ |
|           3 |     0 | delete           |   1 | Hello                    |
|           4 |     1 | update_preimage  |   2 | DuckLake                 |
|           4 |     1 | update_postimage |   2 | DuckLakeDuckLakeDuckLake |

##### Changes Made in the Last Week {#docs:stable:duckdb:advanced_features:data_change_feed::changes-made-in-the-last-week}

```sql
FROM changes.table_changes('tbl', now() - INTERVAL '1 week', now());
```

#### `table_changes` {#docs:stable:duckdb:advanced_features:data_change_feed::table_changes}

The `table_changes` function takes as input the table for which changes should be returned, and two bounds: the start snapshot and the end snapshot (inclusive).
The bounds can be given either as a [snapshot id](#docs:stable:duckdb:usage:snapshots), or as a timestamp.

The result of the function is the set of changes, read using the table schema as of the end snapshot provided, and three extra columns: `snapshot_id`, `rowid` and `change_type`.

| Column        | Description                                                 |
| ------------- | ----------------------------------------------------------- |
| `snapshot_id` | The snapshot which made the change                          |
| `rowid`       | The row identifier of the row which was changed             |
| `change_type` | `insert`, `update_preimage`, `update_postimage` or `delete` |

Updates are split into two rows: the `update_preimage` and `update_postimage`.
`update_preimage` is the row as it was prior to the update operation.
`update_postimage` is the row as it is after the update operation.

When the schema of a table is altered, changes are read as of the schema of the table as of the end snapshot.
As such, if a column is dropped in between the provided bounds, the dropped column is omitted from the entire result.
If a column is added, any changes made to the table prior to the addition of the column will have the column substituted with its default value.

#### `table_deletions` {#docs:stable:duckdb:advanced_features:data_change_feed::table_deletions}

The `table_deletions` function returns only the rows that were *deleted* between two snapshots.
It has the same signature as `table_changes`: it takes a table name and two bounds (start and end snapshot, inclusive), which can be given as snapshot ids or timestamps.

```sql
FROM db.table_deletions('tbl', 3, 3);
```



| snapshot_id | rowid | id |  val  |
| ----------: | ----: | -: | ----- |
|           3 |     0 |  1 | Hello |

The result contains `snapshot_id` and `rowid` columns followed by the data columns of the table.
There is no `change_type` column because all rows returned are deletions.

`table_deletions` is an alias for the `ducklake_table_deletions` function.

#### `table_insertions` {#docs:stable:duckdb:advanced_features:data_change_feed::table_insertions}

The `table_insertions` function returns only the rows that were *inserted* between two snapshots.
It has the same signature as `table_changes`.

```sql
FROM db.table_insertions('tbl', 2, 2);
```



| snapshot_id | rowid |  id | val      |
| ----------: | ----: | --: | -------- |
|           2 |     0 |   1 | Hello    |
|           2 |     1 |   2 | DuckLake |

`table_insertions` is an alias for the `ducklake_table_insertions` function.

#### Compaction {#docs:stable:duckdb:advanced_features:data_change_feed::compaction}

Compaction operations that expire snapshots can limit the change feed that can be read.
For example, if deleted rows are removed as part of compaction, these cannot be returned by the change feed anymore.

### Data Inlining {#docs:stable:duckdb:advanced_features:data_inlining}

When writing small changes to DuckLake, it can be wasteful to write each changeset to an individual Parquet file.
DuckLake supports directly writing small changes to the metadata using _data inlining_.
Instead of writing a Parquet file to the data storage and then writing a reference to that file in the metadata catalog, DuckLake writes the data directly to tables within the metadata catalog.

Data inlining applies to both inserts and deletes:

* **Insertion inlining** 鈥� small inserts are written to a per-table inlined data table instead of a new Parquet data file.
* **Deletion inlining** 鈥� small deletes from existing Parquet data files are written to a per-table inlined deletion table instead of a new Parquet delete file.

Inlined data behaves exactly the same as data written to Parquet files. The only difference is that it lives in the metadata catalog rather than in Parquet files in the data path.

#### Configuring Data Inlining {#docs:stable:duckdb:advanced_features:data_inlining::configuring-data-inlining}

Data inlining is enabled by default with a row limit of 10. Any inserts or deletes that affect fewer rows than `data_inlining_row_limit` are automatically written to inlined tables instead of Parquet files.

##### Global Default {#docs:stable:duckdb:advanced_features:data_inlining::global-default}

The global default row limit is controlled by the `ducklake_default_data_inlining_row_limit` DuckDB setting. This applies to all DuckLake connections that do not have an explicit `data_inlining_row_limit` configured:

```sql
-- Change the default row limit to 50 rows
SET ducklake_default_data_inlining_row_limit = 50;

-- Disable data inlining by default
SET ducklake_default_data_inlining_row_limit = 0;
```

##### Per-Connection Override {#docs:stable:duckdb:advanced_features:data_inlining::per-connection-override}

The `DATA_INLINING_ROW_LIMIT` parameter of the `ATTACH` statement overrides the default for a single connection. This setting is not persisted and must be specified on each attach:

```sql
ATTACH 'ducklake:inlining.duckdb' AS my_ducklake (DATA_INLINING_ROW_LIMIT 10);
```

##### Persistent Override {#docs:stable:duckdb:advanced_features:data_inlining::persistent-override}

The `data_inlining_row_limit` option can be persisted in the DuckLake metadata at the table, schema or global level. A persisted value takes priority over both the global DuckDB default and the ATTACH parameter:

```sql
ATTACH 'ducklake:inlining.duckdb' AS my_ducklake;
USE my_ducklake;
CREATE TABLE t (a INT);
CALL my_ducklake.set_option('data_inlining_row_limit', 10, table_name => 't');
```

#### Insertion Inlining {#docs:stable:duckdb:advanced_features:data_inlining::insertion-inlining}

When insertion inlining is enabled, small inserts are stored directly in the metadata catalog instead of creating a new Parquet data file. DuckLake automatically creates and manages a per-table inlined data table with the following structure:

```sql
-- created and managed internally by DuckLake; one table per schema version
ducklake_inlined_data_鉄╰able_id鉄鉄╯chema_version鉄� (
    row_id         BIGINT,
    begin_snapshot BIGINT,
    end_snapshot   BIGINT,
    -- one column per table column, matching the table schema
    ...
)
```

`begin_snapshot` is the snapshot in which the row was inserted. `end_snapshot` is the snapshot in which the row was deleted, or `NULL` if the row is still live. Deletions of inlined rows are recorded by setting `end_snapshot` rather than creating a separate inlined deletion entry. A new inlined data table is created each time the table schema changes (e.g., a column is added or dropped), so the column layout always matches the current schema.

For example, when inserting a low number of rows, data is automatically inlined:

```sql
ATTACH 'ducklake:inlining.duckdb' AS my_ducklake (DATA_INLINING_ROW_LIMIT 10);
USE my_ducklake;

CREATE TABLE tbl (col INTEGER);
-- Inserting 3 rows 鈥� below the threshold, data is inlined
INSERT INTO tbl VALUES (1), (2), (3);
-- No Parquet files exist
SELECT count(*) FROM glob('inlining.duckdb.files/**');
```

```text
鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹�
鈹� count_star() 鈹�
鈹�    int64     鈹�
鈹溾攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹�
鈹�      0       鈹�
鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹�
```

When inserting more rows than the `DATA_INLINING_ROW_LIMIT`, inserts are automatically written to Parquet:

```sql
INSERT INTO tbl FROM range(100);
SELECT count(*) FROM glob('inlining.duckdb.files/**');
```

```text
鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹�
鈹� count_star() 鈹�
鈹�    int64     鈹�
鈹溾攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹�
鈹�      1       鈹�
鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹�
```

#### Deletion Inlining {#docs:stable:duckdb:advanced_features:data_inlining::deletion-inlining}

When deletion inlining is enabled, deletes from existing Parquet data files that affect fewer rows than `DATA_INLINING_ROW_LIMIT` are stored directly in the metadata catalog instead of creating a Parquet delete file. DuckLake automatically creates and manages a per-table inlined deletion table with the following structure:

```sql
-- created and managed internally by DuckLake; one table per DuckLake table
ducklake_inlined_delete_鉄╰able_id鉄� (
    file_id        BIGINT,
    row_id         BIGINT,
    begin_snapshot BIGINT
)
```

`file_id` is the ID of the Parquet data file containing the deleted row, `row_id` is the position of the deleted row within that file, and `begin_snapshot` is the snapshot in which the deletion occurred.

> **Note** Deletion inlining only applies to rows in existing Parquet data files. Deletes that target inlined insert rows are handled by setting `end_snapshot` on the inlined insert row and do not create an inlined deletion entry.

For example, with deletion inlining enabled, a small delete produces no new Parquet delete file:

```sql
ATTACH 'ducklake:inlining.duckdb' AS my_ducklake (DATA_INLINING_ROW_LIMIT 10);
USE my_ducklake;

CREATE TABLE tbl (col INTEGER);
-- Insert enough rows to exceed the threshold, so they go to Parquet
INSERT INTO tbl FROM range(100);

-- Delete 2 rows 鈥� below the threshold, stored inline
DELETE FROM tbl WHERE col < 2;
-- No new delete file is created
SELECT count(*) FROM glob('inlining.duckdb.files/**/*.parquet') WHERE file LIKE '%-delete.parquet';
```

#### ALTER TABLE Support {#docs:stable:duckdb:advanced_features:data_inlining::alter-table-support}

The following `ALTER TABLE` operations are supported within a transaction that also contains inlined data:

* `ADD COLUMN`
* `DROP COLUMN`
* `RENAME TABLE`
* `RENAME COLUMN`
* `ALTER COLUMN TYPE`
* `SET NOT NULL` / `DROP NOT NULL`

#### Metadata Catalog Support {#docs:stable:duckdb:advanced_features:data_inlining::metadata-catalog-support}

Data inlining is supported when using DuckDB, PostgreSQL or SQLite as the metadata catalog.

When using a non-DuckDB metadata catalog, nested types (` STRUCT`, `MAP` and `LIST`) are stored as `VARCHAR` strings in the inlined data table. DuckLake automatically casts the values back to the correct type when reading.

Inlining of `VARIANT` columns is only supported when using DuckDB as the metadata catalog. PostgreSQL and SQLite cannot inline `VARIANT` values because the type does not round-trip through a string representation without losing type information. Tables with `VARIANT` columns will not have their data inlined when using a non-DuckDB metadata catalog.

#### Flushing Inlined Data {#docs:stable:duckdb:advanced_features:data_inlining::flushing-inlined-data}

Inlined data 鈥� both inlined inserts and inlined deletions 鈥� can be manually flushed to Parquet files by calling the `ducklake_flush_inlined_data` function.

Flush all inlined data in all schemas and tables:

```sql
CALL ducklake_flush_inlined_data('my_ducklake');
```

Flush inlined data only within a specific schema:

```sql
CALL ducklake_flush_inlined_data(
    'my_ducklake',
    schema_name => 'my_schema'
);
```

Flush inlined data for a specific table in the default `main` schema:

```sql
CALL ducklake_flush_inlined_data(
    'my_ducklake',
    table_name => 'my_table'
);
```

Flush inlined data for a specific table in a specific schema:

```sql
CALL ducklake_flush_inlined_data(
    'my_ducklake',
    schema_name => 'my_schema',
    table_name => 'my_table'
);
```

##### Return Values {#docs:stable:duckdb:advanced_features:data_inlining::return-values}

`ducklake_flush_inlined_data` returns one row per table that had data flushed, with the following columns:

| Column         | Type      | Description                                            |
| -------------- | --------- | ------------------------------------------------------ |
| `schema_name`  | `VARCHAR` | Name of the schema containing the table                |
| `table_name`   | `VARCHAR` | Name of the table                                      |
| `rows_flushed` | `BIGINT`  | Number of rows flushed from inlined storage to Parquet |

Tables with no inlined data are not included in the result. Example:

```sql
SELECT schema_name, table_name, rows_flushed
FROM ducklake_flush_inlined_data('my_ducklake');
```

##### Time Travel and Deletions {#docs:stable:duckdb:advanced_features:data_inlining::time-travel-and-deletions}

When flushing inlined inserts that have had rows deleted, DuckLake creates both the materialized Parquet data file and a *partial deletion file*. Rather than creating one deletion file per delete snapshot, the partial deletion file consolidates all deletions into a single Parquet file with an extra column that records the snapshot in which each row was deleted. This preserves full time-travel support while keeping the number of files minimal.

When flushing inlined deletions (rows deleted from existing Parquet data files), DuckLake writes a Parquet delete file for each affected data file. If a delete file already exists for that data file, the inlined deletions are merged into it. The resulting file is always a partial deletion file so that snapshot information is preserved for time-travel queries.

For example:

```sql
ATTACH 'ducklake:inlining.duckdb' AS my_ducklake (DATA_PATH 'data/', DATA_INLINING_ROW_LIMIT 10);
USE my_ducklake;

CREATE TABLE t1 (a INTEGER);
INSERT INTO t1 VALUES (1), (2), (3), (4), (5), (6), (7), (8);

DELETE FROM t1 WHERE a = 2;
DELETE FROM t1 WHERE a = 5;

-- Flush materializes data to Parquet and creates a deletion file with snapshot information
CALL ducklake_flush_inlined_data('ducklake');
```

After flushing, time travel to snapshots before the deletions still returns the deleted rows:

```sql
-- Returns all 8 original rows
SELECT * FROM t1 AT (VERSION => 1);
```

##### Sorted Flush {#docs:stable:duckdb:advanced_features:data_inlining::sorted-flush}

If a table has a [sort order defined](#docs:stable:duckdb:advanced_features:sorted_tables), `ducklake_flush_inlined_data` sorts the output by those keys before writing the resulting Parquet file. The sort order applied is the one currently active on the table at the time of the flush.

##### Interaction with `auto_compact` {#docs:stable:duckdb:advanced_features:data_inlining::interaction-with-auto_compact}

If a table has `auto_compact` set to `false`, `ducklake_flush_inlined_data` will skip it when flushing the whole lake or a whole schema. The same applies when flushing via `CHECKPOINT`, since `CHECKPOINT` calls `ducklake_flush_inlined_data` internally.

> **Note** `auto_compact` does not cause flushing to happen automatically after writes. It only determines whether a table is eligible for flushing when a maintenance function is called.

### Encryption {#docs:stable:duckdb:advanced_features:encryption}

DuckLake supports an encrypted mode.
In this mode, all files that are written to the data directory are encrypted using [Parquet encryption](https://parquet.apache.org/docs/file-format/data-pages/encryption/).
In order to use this mode, the `ENCRYPTED` flag must be passed when initializing the DuckLake catalog:

```sql
ATTACH 'ducklake:encrypted.ducklake'
    (DATA_PATH 'untrusted_location/', ENCRYPTED);
```

When enabled, all Parquet files that are written as part of DuckLake operations are automatically encrypted.
The encryption keys for each file are automatically generated by the system when the files are written.
New encryption keys are automatically generated for each write operation 鈥� such that each file is encrypted using their own encryption key.
The generated keys are stored in the catalog, in the `encryption_key` field of the [`ducklake_data_file`](#docs:stable:specification:tables:ducklake_data_file) table.

When data is read from the encrypted files, the keys are read from the catalog server and automatically used to decrypt the files.
This allows encrypted DuckLake databases to be interacted with in exactly the same manner as unencrypted databases.

### Partitioning {#docs:stable:duckdb:advanced_features:partitioning}

DuckLake tables can be partitioned by a user-defined set of partition keys.
When a DuckLake table has partitioning keys defined, any new data is split up into separate data files along the partitioning keys.
During query planning, the partitioning keys are used to prune which files are scanned.

The partitioning keys defined on a table only affect new data written to the table.
Previously written data will be kept partitioned by the keys the table had when that data was written.
This allows the partition layout of a table to evolve over-time as needed.

The partitioning keys for a file are stored in DuckLake.
These keys do not need to be necessarily stored within the files, or in the paths to the files.

#### Examples {#docs:stable:duckdb:advanced_features:partitioning::examples}

> By default, DuckLake uses [Hive partitioning](https://duckdb.org/docs/current/data/partitioning/hive_partitioning).
> If you want to avoid this style of partitions, you can opt out globally via `CALL my_ducklake.set_option('hive_file_pattern', false)`, or per schema/table by passing the `schema` or `table_name` argument.

Set the partitioning keys of a table, such that new data added to the table is partitioned by these keys.

To partition on a column, use:

```sql
ALTER TABLE tbl SET PARTITIONED BY (part_key);
```

You can also partition using functions. For example, to partition based on the year/month of a timestamp, use:

```sql
ALTER TABLE tbl SET PARTITIONED BY (year(ts), month(ts));
```

To distribute rows into a fixed number of buckets using an Iceberg-compatible hash (Murmur3), use the `bucket` transform:

```sql
ALTER TABLE tbl SET PARTITIONED BY (bucket(8, user_id));
```

Bucket partitioning can be combined with other transforms:

```sql
ALTER TABLE tbl SET PARTITIONED BY (bucket(8, user_id), month(ts));
```

Remove the partitioning keys of a table, such that new data added to the table is no longer partitioned.

```sql
ALTER TABLE tbl RESET PARTITIONED BY;
```

DuckLake supports the following partition clauses:



| Transform | Expression             |
|-----------|------------------------|
| identity  | col_name               |
| bucket    | bucket(N, col_name)    |
| year      | year(ts)               |
| month     | month(ts)              |
| day       | day(ts)                |
| hour      | hour(ts)               |

### Transactions {#docs:stable:duckdb:advanced_features:transactions}

DuckLake has full support for [ACID](https://en.wikipedia.org/wiki/ACID) and offers snapshot isolation for all interactions with the database.
All operations, including DDL statements such as `CREATE TABLE` or `ALTER TABLE`, have full transactional support.
Transactions have all-or-nothing semantics and can be composed of multiple changes that are made to the database.

The extension also provides some syntax to be able to manage transactions. This is explained in the [DuckDB documentation](https://duckdb.org/docs/current/sql/statements/transactions). Basically it comes down to this:

```sql
BEGIN TRANSACTION;
-- Some operation
-- Some other operation
COMMIT;
-- Or
ROLLBACK; -- ABORT will have the same behavior
```

In the context of DuckLake, one committed transaction (i.e., a `BEGIN-COMMIT` block) represents one [snapshot](#docs:stable:duckdb:usage:snapshots).

If multiple transactions are being performed concurrently in one table, the `ducklake` extension has some default configurations for a retry mechanism. This default configurations can be [overridden](#docs:stable:duckdb:usage:configuration).

### Row Lineage {#docs:stable:duckdb:advanced_features:row_lineage}

Every row created in DuckLake has a unique row identifier, which can be queried as the `rowid` virtual column.
This identifier is assigned when a row is first inserted into the system.
The identifier is preserved when the row is moved between files 鈥� for example as part of `UPDATE` and compaction operations.

The `rowid` column can be used to track whether the addition of files actually introduces new rows into DuckLake, or whether the operation is simply moving files around.
This is used internally in the [data change feed](#docs:stable:duckdb:advanced_features:data_change_feed) to differentiate between update operations and delete + insert operations.

### Macros {#docs:stable:duckdb:advanced_features:macros}

DuckLake supports both scalar and table macros. Macros can be created using the standard [`CREATE MACRO` syntax](https://duckdb.org/docs/current/sql/statements/create_macro).

Macros are stored in the DuckLake metadata across three catalog tables: [`ducklake_macro`](#docs:stable:specification:tables:ducklake_macro), [`ducklake_macro_impl`](#docs:stable:specification:tables:ducklake_macro_impl), and [`ducklake_macro_parameters`](#docs:stable:specification:tables:ducklake_macro_parameters).

Macros support [time travel](#docs:stable:duckdb:usage:time_travel), so attaching at a previous snapshot will reflect the macros that existed at that point in time.

#### Scalar Macros {#docs:stable:duckdb:advanced_features:macros::scalar-macros}

Scalar macros return a single value.

```sql
CREATE MACRO add_values(a, b) AS a + b;
SELECT add_values(40, 2);
```

#### Table Macros {#docs:stable:duckdb:advanced_features:macros::table-macros}

Table macros return a table.

```sql
CREATE MACRO filtered_table(threshold) AS TABLE
    SELECT *
    FROM my_table
    WHERE value > threshold;
SELECT * FROM filtered_table(100);
```

#### Default Parameters {#docs:stable:duckdb:advanced_features:macros::default-parameters}

Macros can define default values for parameters.

```sql
CREATE MACRO add_with_default(a, b := 10) AS a + b;
SELECT add_with_default(5);
```

#### Typed Parameters {#docs:stable:duckdb:advanced_features:macros::typed-parameters}

Macros can specify types for their parameters.

```sql
CREATE MACRO typed_add(a INTEGER, b INTEGER) AS a + b;
```

#### Dropping Macros {#docs:stable:duckdb:advanced_features:macros::dropping-macros}

Macros can be dropped using `DROP MACRO` or `DROP MACRO TABLE` for table macros.

```sql
DROP MACRO add_values;
DROP MACRO TABLE filtered_table;
```

### Views {#docs:stable:duckdb:advanced_features:views}

Views can be created using the standard [`CREATE VIEW` syntax](https://duckdb.org/docs/current/sql/statements/create_view).
The views are stored in the metadata, in the [`ducklake_view`](#docs:stable:specification:tables:ducklake_view) table.

#### Examples {#docs:stable:duckdb:advanced_features:views::examples}

Create a view.

```sql
CREATE VIEW v1 AS SELECT * FROM tbl;
```

### Comments {#docs:stable:duckdb:advanced_features:comments}

Comments can be added to tables, views and columns using the [`COMMENT ON`](https://duckdb.org/docs/current/sql/statements/comment_on) syntax.
The comments are stored in the metadata, and can be modified in a transactional manner:
- For tables and views, comments are in the [`ducklake_tag`](#docs:stable:specification:tables:ducklake_tag) table.
- For columns, comments are in the [`ducklake_column_tag`](#docs:stable:specification:tables:ducklake_column_tag) table.

#### Examples {#docs:stable:duckdb:advanced_features:comments::examples}

Create a comment on a `TABLE`:

```sql
COMMENT ON TABLE test_table IS 'very nice table';
```

Create a comment on a `COLUMN`:

```sql
COMMENT ON COLUMN test_table.test_table_column IS 'very nice column';
```

### Sorted Tables {#docs:stable:duckdb:advanced_features:sorted_tables}

DuckLake tables can be configured with a sort order. When a sort order is defined, data is physically sorted by the specified columns whenever it is written out as Parquet 鈥� during `INSERT`, [file compaction](#docs:stable:duckdb:maintenance:merge_adjacent_files) and [inlined data flushing](#docs:stable:duckdb:advanced_features:data_inlining).

Sorting data before writing improves the effectiveness of min/max statistics at query time, which allows the DuckDB query engine to skip data files whose value ranges do not overlap with a query's filter predicates.

#### Example Setup {#docs:stable:duckdb:advanced_features:sorted_tables::example-setup}

The examples on this page use the following DuckLake instance:

```sql
ATTACH 'ducklake:sorted.duckdb' AS my_ducklake (DATA_PATH 'data/');
USE my_ducklake;
CREATE TABLE events (event_time TIMESTAMP, event_type VARCHAR, value DOUBLE);
```

#### Setting a Sort Order {#docs:stable:duckdb:advanced_features:sorted_tables::setting-a-sort-order}

Set the sort order for a table using `SET SORTED BY`:

```sql
ALTER TABLE events SET SORTED BY (event_time ASC);
```

Multiple sort keys are supported:

```sql
ALTER TABLE events SET SORTED BY (event_time ASC, event_type DESC);
```

`ASC` and `DESC` control the sort direction. `NULLS FIRST` and `NULLS LAST` are also supported to control null ordering:

```sql
ALTER TABLE events SET SORTED BY (event_time ASC NULLS LAST);
```

##### Expression-Based Sort Keys {#docs:stable:duckdb:advanced_features:sorted_tables::expression-based-sort-keys}

Arbitrary expressions are supported in `SET SORTED BY`, not just column references. This includes function calls, casts, and [DuckLake macros](#docs:stable:duckdb:advanced_features:macros).

Sort by the hour extracted from a timestamp:

```sql
ALTER TABLE events SET SORTED BY (date_trunc('hour', event_time) ASC);
```

Sort by a DuckLake macro:

```sql
CREATE MACRO event_bucket(t) AS date_trunc('day', t);
ALTER TABLE events SET SORTED BY (event_bucket(event_time) ASC);
```

Expressions are validated when `SET SORTED BY` is executed 鈥� an error is returned if any referenced columns or functions cannot be resolved.

#### Removing a Sort Order {#docs:stable:duckdb:advanced_features:sorted_tables::removing-a-sort-order}

To remove the sort order from a table, use `RESET SORTED BY`:

```sql
ALTER TABLE events RESET SORTED BY;
```

After resetting, subsequent compactions and flushes will write data without sorting.

#### Effect on Insert {#docs:stable:duckdb:advanced_features:sorted_tables::effect-on-insert}

By default, `INSERT` statements automatically sort data according to the table's sort order before writing Parquet files. This behavior is controlled by the [`sort_on_insert`](#docs:stable:duckdb:usage:configuration) option, which defaults to `true`.

To disable sorting on insert (e.g., when insertion speed is the primary concern):

```sql
CALL my_ducklake.set_option('sort_on_insert', false, table_name => 'events');
```

When `sort_on_insert` is disabled, data written to Parquet during compaction and inlined data flushing is still sorted according to the table's sort order.

##### Interaction with Data Inlining {#docs:stable:duckdb:advanced_features:sorted_tables::interaction-with-data-inlining}

When [data inlining](#docs:stable:duckdb:advanced_features:data_inlining) is enabled and `sort_on_insert` is `false`, data that exceeds the inlining row limit is still sorted before being written to Parquet. Inlined data (which stays in the metadata catalog) is not sorted at insert time 鈥� it will be sorted when it is flushed.

#### Effect on Compaction and Flush {#docs:stable:duckdb:advanced_features:sorted_tables::effect-on-compaction-and-flush}

Once a sort order is set, the **current** sort order is applied at the time of compaction or flush 鈥� not the sort order that was active when the source data was written.

When `ducklake_merge_adjacent_files` runs on a sorted table, the merged output file is sorted:

```sql
ALTER TABLE events SET SORTED BY (event_time ASC);
CALL ducklake_merge_adjacent_files('my_ducklake', 'events');
```

The same applies when flushing inlined data:

```sql
CALL ducklake_flush_inlined_data('my_ducklake', table_name => 'events');
```

### Logging {#docs:stable:duckdb:advanced_features:logging}

DuckDB provides a [logging framework](https://duckdb.org/docs/current/operations_manual/logging/overview) that can be used to debug and monitor DuckLake operations. The DuckLake extension registers a dedicated log type for metadata queries, and DuckDB's built-in `QueryLog` type can be used to trace all SQL queries including those issued internally by the extension.

#### DuckLake Metadata Query Log {#docs:stable:duckdb:advanced_features:logging::ducklake-metadata-query-log}

Metadata queries can be an important factor in performance, especially when using a remote catalog (e.g., PostgreSQL). The DuckLake extension registers the `DuckLakeMetadata` log type, which logs every metadata query together with timing information.

##### Enabling the Log {#docs:stable:duckdb:advanced_features:logging::enabling-the-log}

```sql
CALL enable_logging('DuckLakeMetadata');
```

##### Log Fields {#docs:stable:duckdb:advanced_features:logging::log-fields}

`DuckLakeMetadata` is a structured log type. Use `duckdb_logs_parsed` to query the individual fields directly:

| Field        | Type      | Description                                                           |
| ------------ | --------- | --------------------------------------------------------------------- |
| `catalog`    | `VARCHAR` | The name of the DuckLake catalog that issued the query                |
| `query`      | `VARCHAR` | The metadata SQL query that was executed against the catalog database |
| `elapsed_ms` | `BIGINT`  | The time the query took to execute, in milliseconds                   |

##### Example {#docs:stable:duckdb:advanced_features:logging::example}

```sql
ATTACH 'ducklake:catalog.db' AS my_ducklake (DATA_PATH 'data/');
USE my_ducklake;
CREATE TABLE t1 (a INTEGER, b VARCHAR);

-- Enable DuckLake metadata logging
CALL enable_logging('DuckLakeMetadata');

-- Run an operation that triggers metadata queries
INSERT INTO t1 VALUES (1, 'hello'), (2, 'world');

-- View the metadata queries with structured fields
SELECT catalog, elapsed_ms, query
FROM duckdb_logs_parsed('DuckLakeMetadata');
```

#### DuckDB Query Log {#docs:stable:duckdb:advanced_features:logging::duckdb-query-log}

DuckDB's built-in `QueryLog` type logs every query executed on every connection, including the internal queries that the DuckLake extension issues against the catalog database. This can be useful for general debugging or to see the full picture of what DuckLake does under the hood.

##### Enabling the Log {#docs:stable:duckdb:advanced_features:logging::enabling-the-log}

```sql
CALL enable_logging('QueryLog');
```

##### Example {#docs:stable:duckdb:advanced_features:logging::example}

```sql
ATTACH 'ducklake:catalog.db' AS my_ducklake (DATA_PATH 'data/');
USE my_ducklake;
CREATE TABLE t1 (a INTEGER, b VARCHAR);

-- Enable query logging
CALL enable_logging('QueryLog');

-- Run an operation
INSERT INTO t1 VALUES (1, 'hello');

-- View all queries including internal DuckLake metadata queries
SELECT type, message
FROM duckdb_logs
WHERE type = 'QueryLog';
```

#### Combining Log Types {#docs:stable:duckdb:advanced_features:logging::combining-log-types}

Both log types can be enabled simultaneously to get DuckLake-specific timing information alongside the full query trace:

```sql
CALL enable_logging(['DuckLakeMetadata', 'QueryLog']);
```

#### Logging to Different Storages {#docs:stable:duckdb:advanced_features:logging::logging-to-different-storages}

By default, logs are stored in an in-memory buffer and queried via the `duckdb_logs` view. DuckDB also supports logging to stdout or to a file:

```sql
-- Log to stdout
CALL enable_logging('DuckLakeMetadata', storage = 'stdout');

-- Log to a file
CALL enable_logging('DuckLakeMetadata', storage_path = '/tmp/ducklake_logs');
```

For more details on log storages and advanced configuration, see the [DuckDB Logging documentation](https://duckdb.org/docs/current/operations_manual/logging/overview).

## Metadata {#duckdb:metadata}

### List Files {#docs:stable:duckdb:metadata:list_files}

The `ducklake_list_files` function can be used to list the data files and corresponding delete files that belong to a given table, optionally for a given snapshot.

#### Usage {#docs:stable:duckdb:metadata:list_files::usage}

List all files:

```sql
FROM ducklake_list_files('catalog', 'table_name');
```

Get list of files at a specific snapshot version:

```sql
FROM ducklake_list_files('catalog', 'table_name', snapshot_version => 2);
```

Get list of files at a specific point in time:

```sql
FROM ducklake_list_files('catalog', 'table_name', snapshot_time => '2025-06-16 15:24:30');
```

Get list of files of a table in a specific schema:

```sql
FROM ducklake_list_files('catalog', 'table_name', schema => 'main');
```

| Parameter          | Description                                      | Default |
| ------------------ | ------------------------------------------------ | ------- |
| `catalog`          | Name of attached DuckLake catalog                |         |
| `table_name`       | Name of table to fetch files from                |         |
| `schema`           | Schema for the table                             | `main`  |
| `snapshot_version` | If provided, fetch files for a given snapshot id |         |
| `snapshot_time`    | If provided, fetch files for a given timestamp   |         |

#### Result {#docs:stable:duckdb:metadata:list_files::result}

The function returns the following result set.



| column_name                | column_type |
| -------------------------- | ----------- |
| data_file                  | VARCHAR     |
| data_file_size_bytes       | UBIGINT     |
| data_file_footer_size      | UBIGINT     |
| data_file_encryption_key   | BLOB        |
| delete_file                | VARCHAR     |
| delete_file_size_bytes     | UBIGINT     |
| delete_file_footer_size    | UBIGINT     |
| delete_file_encryption_key | BLOB        |

* If the file has delete files, the corresponding delete file is returned, otherwise these fields are `NULL`.
* If the database is encrypted, the encryption key must be used to read the file.
* The `footer_size` refers to the Parquet footer size 鈥� this is optionally provided.

### Adding Files {#docs:stable:duckdb:metadata:adding_files}

The `ducklake_add_data_files` function can be used to register existing data files as new files in DuckLake.
The files are not copied over 鈥� DuckLake is merely made aware of their existence, allowing them to be queried through DuckLake.
Adding files in this manner supports regular transactional semantics.

> The ownership of the Parquet file is transferred to DuckLake. As such, compaction operations (such as those triggered through `merge_adjacent_files` or `expire_snapshots` followed by `cleanup_old_files`) can cause the files to be deleted by DuckLake.

#### Usage {#docs:stable:duckdb:metadata:adding_files::usage}

##### Examples {#docs:stable:duckdb:metadata:adding_files::examples}

Add the file `people.parquet` to the `people` table in `my_ducklake`:

```sql
CALL ducklake_add_data_files('my_ducklake', 'people', 'people.parquet');
```

Target a specific schema rather than the default `main`:

```sql
CALL ducklake_add_data_files('my_ducklake', 'people', 'people.parquet', schema => 'some_schema');
```

Add the file. Any columns that are present in the table but not in the file will have their default values used when reading:

```sql
CALL ducklake_add_data_files('my_ducklake', 'people', 'people.parquet', allow_missing => true);
```

Add the file. If the file has extra columns in the table they will be ignored (they will not be queryable through DuckLake):

```sql
CALL ducklake_add_data_files('my_ducklake', 'people', 'people.parquet', ignore_extra_columns => true);
```

##### Missing Columns {#docs:stable:duckdb:metadata:adding_files::missing-columns}

When adding files to a table, all columns that are present in the table must be present in the Parquet file, otherwise an error is thrown.
The `allow_missing` option can be used to add the file anyway 鈥� in which case any missing columns will be substituted with the `initial_default` value of the column.

##### Extra Columns {#docs:stable:duckdb:metadata:adding_files::extra-columns}

When adding files to a table, if there are any columns present that are not present in the table, an error is thrown by default.
The `ignore_extra_columns` option can be used to add the file anyway 鈥� any extra columns will be ignored and unreachable.

#### Type Mapping {#docs:stable:duckdb:metadata:adding_files::type-mapping}

In general, types of columns in the source Parquet file must match the type as defined in the table, otherwise an error is thrown. Types in the Parquet file can be narrower than the type defined in the table. Below is a supported mapping type:

| Table type      | Supported Parquet types                         |
| --------------- | ----------------------------------------------- |
| `bool`          | `bool`                                          |
| `int8`          | `int8`                                          |
| `int16`         | `int[8/16]`, `uint8`                            |
| `int32`         | `int[8/16/32]`, `uint[8/16]`                    |
| `int64`         | `int[8/16/32/64]`, `uint[8/16/32]`              |
| `uint8`         | `uint8`                                         |
| `uint16`        | `uint[8/16]`                                    |
| `uint32`        | `uint[8/16/32]`                                 |
| `uint64`        | `uint[8/16/32/64]`                              |
| `float`         | `float`                                         |
| `double`        | `float/double`                                  |
| `decimal(P, S)` | `decimal(P',S')`, where `P' <= P `and `S' <= S` |
| `blob`          | `blob`                                          |
| `varchar`       | `varchar`                                       |
| `date`          | `date`                                          |
| `time`          | `time`                                          |
| `timestamp`     | `timestamp`, `timestamp_ns`                     |
| `timestamp_ns`  | `timestamp`, `timestamp_ns`                     |
| `timestamptz`   | `timestamptz`                                   |

## Migrations {#duckdb:migrations}

### DuckDB to DuckLake {#docs:stable:duckdb:migrations:duckdb_to_ducklake}

Migrating from DuckDB to DuckLake is very simple to do with the DuckDB `ducklake` extension. However, if you are currently using some DuckDB features that are [unsupported in DuckLake](#docs:stable:duckdb:unsupported_features), this guide will definitely help you.

#### First Scenario: Everything Is Supported {#docs:stable:duckdb:migrations:duckdb_to_ducklake::first-scenario-everything-is-supported}

If you are not using any of the unsupported features, migrating from DuckDB to DuckLake will be as simple as running the following commands:

```sql
ATTACH 'ducklake:my_ducklake.ducklake' AS my_ducklake;
ATTACH 'db.duckdb' AS my_duckdb;

COPY FROM DATABASE my_duckdb TO my_ducklake;
```

Note that it doesn't matter what catalog you are using as a metadata backend for DuckLake.

#### Second Scenario: Not Everything Is Supported {#docs:stable:duckdb:migrations:duckdb_to_ducklake::second-scenario-not-everything-is-supported}

If you have been using DuckDB for a while, there is a chance you are using some very specific types, macros, default values that are not literals or even things like generated columns. If this is your case, then migrating will have some tradeoffs.

- Specific types need to be cast to a [supported DuckLake type](#docs:stable:specification:data_types). User defined types that are created as a `STRUCT` can be interpreted as such and `ENUM` and `UNION` will be cast to `VARCHAR` and `VARINT` will be cast to `INT`.

- Macros can be migrated to a DuckDB persisted database. If you are using DuckDB as your catalog for DuckLake, then this will be the destination. If you are using other catalogs like PostgreSQL, SQLite or MySQL, DuckDB macros are not supported and therefore can't be migrated.

- Generated columns are the same as defaults that are not literals and therefore they need to be specified when inserting the data into the destination table. This means that the values will always be persisted (no `VIRTUAL` option).

##### Migration Script {#docs:stable:duckdb:migrations:duckdb_to_ducklake::migration-script}

The following Python script can be used to migrate from a DuckDB persisted database to DuckLake bypassing the unsupported features.

> Currently, only local migrations are supported by this script. The script will be adapted in the future to account for migrations to remote object storage such as S3 or GCS.



<details markdown='1'>
<summary markdown='span'>
<span style="text-decoration: underline">Click to see the Python script that migrates from DuckDB to DuckLake.</span>
</summary>
```python
import duckdb
import argparse
import re
import os
from collections import deque

TYPE_MAPPING = {
    "VARINT": "::VARCHAR::INT",
    "UNION/ENUM": "::VARCHAR",
    "BIT": "::VARCHAR",
}


def get_postgres_secret():
    return f"""
        CREATE SECRET postgres_secret(
            TYPE postgres,
            HOST '{os.getenv("POSTGRES_HOST", "localhost")}',
            PORT {os.getenv("POSTGRES_PORT", "5432")},
            DATABASE {os.getenv("POSTGRES_DB", "migration_test")},
            USER '{os.getenv("POSTGRES_USER", "user")}',
            PASSWORD '{os.getenv("POSTGRES_PASSWORD", "simple")}'
        );"""


def _resolve_data_types(
    table: str, schema: str, catalog: str, conn: duckdb.DuckDBPyConnection
):
    excepts = []
    casts = []
    for col in conn.execute(
        f"SELECT column_name, data_type FROM information_schema.columns WHERE table_name = '{table}' AND table_schema = '{schema}' AND table_catalog = '{catalog}'"
    ).fetchall():
        col_name, col_type = col[0], col[1]
        # Handle mapped types
        if col_type in TYPE_MAPPING or re.match(r"(ENUM|UNION)\b", col_type):
            cast = TYPE_MAPPING.get(col_type) or TYPE_MAPPING["UNION/ENUM"]
            casts.append(f"{col_name}{cast} AS {col_name}")
            excepts.append(col_name)
        # Handle array types
        elif re.fullmatch(r"(INTEGER|VARCHAR|FLOAT)\[\d+\]", col_type):
            base_type = re.match(r"(INTEGER|VARCHAR|FLOAT)", col_type).group(1)
            cast = f"::{base_type}[]"
            casts.append(f"{col_name}{cast} AS {col_name}")
            excepts.append(col_name)
    return excepts, casts


def migrate_tables_and_views(duckdb_catalog: str, con: duckdb.DuckDBPyConnection):
    """
    Migrate tables and views from the DuckDB catalog to DuckLake using a queue system.
    If migration of a table or view fails, it will be re-added to the back of the queue.
    """
    rows = con.execute(
        f"SELECT table_catalog, table_schema, table_name, table_type "
        f"FROM information_schema.tables WHERE table_catalog = '{duckdb_catalog}'"
    ).fetchall()

    # The idea behind this queue is to retry failed migration of views due to missing dependencies.
    # The failed item is re-added to the back of the queue and waits for the rest of the dependencies to be migrated.
    # This avoids the need to generate a full dependency graph, which would make this script very complex.
    queue = deque(rows)
    failed_last_round = set()

    while queue:
        catalog, schema, table, table_type = queue.popleft()
        con.execute(f"CREATE SCHEMA IF NOT EXISTS {schema}")
        try:
            if table_type == "VIEW":
                view_definition = con.execute(
                    f"SELECT view_definition FROM information_schema.views "
                    f"WHERE table_name = '{table}' AND table_schema = '{schema}' AND table_catalog = '{catalog}'"
                ).fetchone()[0]
                con.execute(
                    f"CREATE VIEW IF NOT EXISTS {view_definition.removeprefix('CREATE VIEW ')}"
                )
                print(f"Migrating Catalog: {catalog}, Schema: {schema}, View: {table}")
            else:
                excepts, casts = _resolve_data_types(table, schema, catalog, con)
                if casts:
                    select_clause = (
                        "* EXCLUDE(" + ", ".join(excepts) + "),\n" + ",\n".join(casts)
                    )
                    con.execute(
                        f"CREATE TABLE IF NOT EXISTS {schema}.{table} AS "
                        f"SELECT {select_clause} FROM {catalog}.{schema}.{table}"
                    )
                else:
                    con.execute(
                        f"CREATE TABLE IF NOT EXISTS {schema}.{table} AS "
                        f"SELECT * FROM {catalog}.{schema}.{table}"
                    )
                print(f"Migrating Catalog: {catalog}, Schema: {schema}, Table: {table}")
        except Exception as e:
            print(f"WARNING: Requeuing {table_type} {table}")
            # Prevent infinite loop if no progress is possible
            if (catalog, schema, table, table_type) in failed_last_round:
                print(
                    f"Skipping {table_type} {table} permanently due to repeated failure. {e}"
                )
                continue
            else:
                queue.append((catalog, schema, table, table_type))
                failed_last_round.add((catalog, schema, table, table_type))
        else:
            # Success 鈥� ensure we clear from failure set
            failed_last_round.discard((catalog, schema, table, table_type))


def migrate_macros(con: duckdb.DuckDBPyConnection, duckdb_catalog: str):
    """
    Migrate macros from the DuckDB catalog to DuckLake metadata database.
    """
    for row in con.execute(
        f"SELECT function_name, parameters, macro_definition FROM duckdb_functions() "
        f"WHERE database_name='{duckdb_catalog}'"
    ).fetchall():
        name, parameters, definition = row[0], row[1], row[2]
        print(f"Migrating Macro: {name}")
        con.execute(
            f"CREATE OR REPLACE MACRO {name}({','.join(parameters)}) AS {definition}"
        )


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Migrate DuckDB catalog to DuckLake.")
    parser.add_argument("--duckdb-catalog", required=True, help="DuckDB catalog name")
    parser.add_argument("--duckdb-file", required=True, help="Path to DuckDB file")
    parser.add_argument(
        "--ducklake-catalog", required=True, help="DuckLake catalog name"
    )
    parser.add_argument(
        "--catalog-type",
        choices=["duckdb", "postgresql", "sqlite"],
        required=True,
        help="Choose one of: duckdb, postgresql, sqlite",
    )
    parser.add_argument("--ducklake-file", required=False, help="Path to DuckLake file")
    parser.add_argument(
        "--ducklake-data-path", required=True, help="Data path for DuckLake"
    )

    args = parser.parse_args()

    con = duckdb.connect(database=args.duckdb_file)

    if args.catalog_type == "postgresql":
        con.execute(get_postgres_secret())

    secret = (
        "CREATE SECRET ducklake_secret (TYPE ducklake"
        + (
            f"\n,METADATA_PATH '{args.ducklake_file if args.catalog_type == 'duckdb' else f'sqlite:{args.ducklake_file}'}'"
            if args.catalog_type in ("duckdb", "sqlite")
            else "\n,METADATA_PATH ''"
        )
        + f"\n,DATA_PATH '{args.ducklake_data_path}'"
        + (
            "\n,METADATA_PARAMETERS MAP {'TYPE': 'postgres', 'SECRET': 'postgres_secret'});"
            if args.catalog_type == "postgresql"
            else ");"
        )
    )
    con.execute(secret)

    con.execute(
        f"ATTACH '{args.duckdb_file}' AS {args.duckdb_catalog};"
        f"ATTACH 'ducklake:ducklake_secret' AS {args.ducklake_catalog}; USE {args.ducklake_catalog};"
    )

    migrate_tables_and_views(
        duckdb_catalog=args.duckdb_catalog,
        con=con,
    )

    if args.catalog_type == "duckdb":
        # DETACH DuckLake to be able to attach to the metadata database in migrate_macros
        con.execute(f"USE {args.duckdb_catalog}; DETACH {args.ducklake_catalog};")
        con.execute(
            f"ATTACH '{args.ducklake_file}' AS ducklake_metadata; USE ducklake_metadata;"
        )
        migrate_macros(
            con=con,
            duckdb_catalog=args.duckdb_catalog,
        )
    con.close()
```
</details>



The script can be run in any Python environment with DuckDB installed. The usage is the following:

```text
usage: migration.py [-h]
    --duckdb-catalog DUCKDB_CATALOG
    --duckdb-file DUCKDB_FILE
    --ducklake-catalog DUCKLAKE_CATALOG
    --catalog-type {duckdb,postgresql,sqlite}
    [--ducklake-file DUCKLAKE_FILE]
    --ducklake-data-path DUCKLAKE_DATA_PATH
```

If you are migrating to PostgreSQL, make sure that you provide the following environment variables for the PostgreSQL secret connection:

- `POSTGRES_HOST`
- `POSTGRES_PORT`
- `POSTGRES_DB`
- `POSTGRES_USER`
- `POSTGRES_PASSWORD`

## Guides {#duckdb:guides}

### Guides {#docs:stable:duckdb:guides:overview}

The guides section contains compact how-to guides that are focused on achieving a single goal.

### Access Control {#docs:stable:duckdb:guides:access_control}

While access control is not per se a feature of DuckLake, we can leverage the tools that DuckLake uses and their permission systems to implement schema- and table-level permissions in DuckLake.

#### Basic Principles {#docs:stable:duckdb:guides:access_control::basic-principles}

In this guide, we focus on three different roles regarding access control in DuckLake:

- The **DuckLake Superuser** can perform any DuckLake operation, most notably:
  - Initializing DuckLake (done the first time we run the `ATTACH` command).
  - Creating schemas.
  - `CREATE`, `INSERT`, `UPDATE`, `DELETE`, and `SELECT` on any DuckLake table.

- The **DuckLake Writer** can perform the following operations:
  - `ATTACH` to an existing DuckLake.
  - `CREATE`, `INSERT`, `UPDATE`, `DELETE`, and `SELECT` on any or a subset of DuckLake tables.
  - Optionally, `SELECT` on any or a subset of DuckLake tables.
  - Optionally, `CREATE` schema.

- The **DuckLake Reader** can perform the following operations:
  - `ATTACH` to an existing DuckLake. Both `READ_ONLY` and regular attaching modes will work.
  - `SELECT` on any or a subset of DuckLake tables.

> These roles are not actually implemented in DuckLake; they are constructs used in this guide, as they represent the most common types of roles present in data management systems.

DuckLake has two components: the metadata catalog, which resides in a SQL database, and the storage, which can be any filesystem backend. The roles mentioned above require different specific permissions at the **catalog level:**
- The DuckLake Superuser needs all permissions under the specified schema (` public` by default). Since this user initializes all tables, they also become the owner. Subsequent migrations between different version of the DuckLake specification must be carried out by this user.
- The DuckLake Writer only needs permissions to `INSERT`, `UPDATE`, `DELETE`, and `SELECT` at the catalog level. This is sufficient for any operation in DuckLake, including operations that expire snapshots.
- The DuckLake Reader only needs `SELECT` permissions at the catalog level.

At the storage level, we can leverage the way DuckLake structures data paths for different tables, which uses the following convention:

```sql
/鉄╯chema鉄�/鉄╰able鉄�/鉄╬artition鉄�/鉄╠ata_file鉄�.parquet
```

Using this convention and the policy mechanisms of certain filesystems (such as cloud-based object storage), we can establish access to certain paths at the schema, table, or even partition level.

> This will not work if we use `ducklake_add_data_files` and the added files do not follow the path convention; permissions at the path level will not apply to these files.


The following diagram shows how these roles and their necessary permissions work in DuckLake:

![Access control in DuckLake](../images/docs/guides/ducklake_access_control-light.svg)


#### Access Control with S3 and PostgreSQL {#docs:stable:duckdb:guides:access_control::access-control-with-s3-and-postgresql}

The following is an example implementation of the basic principles described above, focusing on PostgreSQL as a DuckLake catalog and S3 as the storage backend.

##### PostgreSQL Requirements {#docs:stable:duckdb:guides:access_control::postgresql-requirements}

In this section, we create the three roles described above in PostgreSQL. We create them as users for simplicity, but you may also create them as groups if you expect a specific role to be used by multiple users.

```sql
-- Setup initialization user, migrations, and writing, assuming the database is already created
CREATE USER ducklake_superuser WITH PASSWORD 'simple';
GRANT CREATE ON DATABASE access_control TO ducklake_superuser;
GRANT CREATE, USAGE ON SCHEMA public TO ducklake_superuser;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO ducklake_superuser;

-- Writer/reader
CREATE USER ducklake_writer WITH PASSWORD 'simple';
GRANT USAGE ON SCHEMA public TO ducklake_writer;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO ducklake_writer;

-- Reader only
CREATE USER ducklake_reader WITH PASSWORD 'simple';
GRANT USAGE ON SCHEMA public TO ducklake_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO ducklake_reader;
```

##### S3 Requirements {#docs:stable:duckdb:guides:access_control::s3-requirements}

In AWS, we create three users. The writer user will only have access to a specific schema, and the reader will only have access to a specific table. The policies needed for these users are as follows:

<details markdown='1'>
<summary markdown='span'>
**DuckLake Superuser**
</summary>
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "DuckLakeSuperuser",
      "Effect": "Allow",
      "Action": [
        "s3:ListBucket",
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject"
      ],
      "Resource": [
        "arn:aws:s3:::ducklake-access-control",
        "arn:aws:s3:::ducklake-access-control/*"
      ]
    }
  ]
}
```
</details>

<details markdown='1'>
<summary markdown='span'>
**DuckLake Writer/Reader**
</summary>
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "DuckLakeWriter",
      "Effect": "Allow",
      "Action": [
        "s3:ListBucket",
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject"
      ],
      "Resource": [
        "arn:aws:s3:::ducklake-access-control",
        "arn:aws:s3:::ducklake-access-control/some_schema/*"
      ]
    }
  ]
}
```

Note that we allow `s3:DeleteObject`, which enables the writer to perform compaction and cleanup jobs that require rewriting data files.
</details>

<details markdown='1'>
<summary markdown='span'>
**DuckLake Reader**
</summary>
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "DuckLakeReader",
      "Effect": "Allow",
      "Action": [
        "s3:GetObject"
      ],
      "Resource": [
        "arn:aws:s3:::ducklake-access-control",
        "arn:aws:s3:::ducklake-access-control/some_schema/some_table/*"
      ]
    }
  ]
}
```
</details>

##### DuckLake Test {#docs:stable:duckdb:guides:access_control::ducklake-test}

In this section, we connect to DuckLake using these different roles to demonstrate how the implementation works in practice using the DuckLake extension of DuckDB.

Let's initialize DuckLake and perform some basic operations with the **DuckLake Superuser**.

```sql
-- Using the credentials for the AWS DuckLake Superuser (other providers such as STS or SSO can also be used)
CREATE OR REPLACE SECRET s3_ducklake_superuser (
  TYPE s3,
  PROVIDER config,
  KEY_ID '鉄╧ey鉄�',
  SECRET '鉄╯ecret鉄�',
  REGION 'eu-north-1'
);

-- Using the DuckLake Superuser credentials for Postgres
CREATE OR REPLACE SECRET postgres_secret_superuser (
  TYPE postgres,
  HOST 'localhost',
  DATABASE 'access_control',
  USER 'ducklake_superuser',
  PASSWORD 'simple'
);

-- DuckLake config secret
CREATE OR REPLACE SECRET ducklake_superuser_secret (
  TYPE ducklake,
  METADATA_PATH '',
  DATA_PATH 's3://ducklake-access-control/',
  METADATA_PARAMETERS MAP {'TYPE': 'postgres','SECRET': 'postgres_secret_superuser'}
);

-- This initializes DuckLake
ATTACH 'ducklake:ducklake_superuser_secret' AS ducklake_superuser;
USE ducklake_superuser;

-- Perform operations in DuckLake
CREATE SCHEMA IF NOT EXISTS some_schema;
CREATE TABLE IF NOT EXISTS some_schema.some_table (id INTEGER, name VARCHAR);
INSERT INTO some_schema.some_table VALUES (1, 'test');
```

Now let's use the **DuckLake Writer:**

```sql
-- Drop this to avoid the extension defaulting to this secret
DROP SECRET s3_ducklake_superuser;

-- Using the DuckLake Writer credentials for Postgres
CREATE OR REPLACE SECRET postgres_secret_writer (
  TYPE postgres,
  HOST 'localhost',
  DATABASE 'access_control',
  USER 'ducklake_writer',
  PASSWORD 'simple'
);

-- Using the credentials for the AWS DuckLake Writer
CREATE OR REPLACE SECRET s3_ducklake_schema_reader_writer (
  TYPE s3,
  PROVIDER config,
  KEY_ID '鉄╧ey鉄�',
  SECRET '鉄╯ecret鉄�',
  REGION 'eu-north-1'
);

-- DuckLake config secret
CREATE OR REPLACE SECRET ducklake_writer_secret (
  TYPE ducklake,
  METADATA_PATH '',
  DATA_PATH 's3://ducklake-access-control/',
  METADATA_PARAMETERS MAP {'TYPE': 'postgres','SECRET': 'postgres_secret_writer'}
);

ATTACH 'ducklake:ducklake_writer_secret' AS ducklake_writer;
USE ducklake_writer;

-- Perform operations
CREATE TABLE IF NOT EXISTS some_schema.another_table (id INTEGER, name VARCHAR);
INSERT INTO some_schema.another_table VALUES (1, 'test'); -- Works
INSERT INTO some_schema.some_table VALUES (2, 'test2'); -- Also works

-- Try to perform an unauthorized operation
CREATE TABLE other_table_in_main (id INTEGER, name VARCHAR); -- This unfortunately works
INSERT INTO other_table_in_main VALUES (1, 'test'); -- This doesn't work
```

In the last example, there are limitations to this approach. We can create an empty table, as this only inserts a new record in the metadata catalog鈥攕omething the DuckLake Writer is allowed to do. The solution is to wrap table initializations in a transaction to ensure the table can't be created if there is no permission to insert data.

```sql
BEGIN TRANSACTION;
CREATE TABLE other_table_in_main (id INTEGER, name VARCHAR);
INSERT INTO other_table_in_main VALUES (1, 'test');
COMMIT;
```

This will throw the following error:

```console
HTTP Error:
Unable to connect to URL "https://ducklake-access-control.s3.amazonaws.com/main/other_table_in_main/ducklake-01992ec2-d9f7-745e-88e8-708e659a70be.parquet": 403 (Forbidden).

Authentication Failure - this is usually caused by invalid or missing credentials.
* No credentials are provided.
* See https://duckdb.org/docs/stable/extensions/httpfs/s3api.html
```

> The error message is the generic one used when DuckDB cannot access an object in S3; nothing specific to DuckLake.

The **DuckLake Reader** is the simplest role.

```sql
DROP SECRET s3_ducklake_schema_reader_writer;
CREATE OR REPLACE SECRET s3_ducklake_table_reader (
  TYPE s3,
  PROVIDER config,
  KEY_ID '鉄╧ey_id鉄�',
  SECRET '鉄╯ecret_key鉄�',
  REGION 'eu-north-1'
);
CREATE OR REPLACE SECRET postgres_secret_reader (
  TYPE postgres,
  HOST 'localhost',
  DATABASE 'access_control',
  USER 'ducklake_reader',
  PASSWORD 'simple'
);
CREATE OR REPLACE SECRET ducklake_reader_secret (
  TYPE ducklake,
  METADATA_PATH '',
  DATA_PATH 's3://ducklake-access-control/',
  METADATA_PARAMETERS MAP {'TYPE': 'postgres','SECRET': 'postgres_secret_reader'}
);
ATTACH 'ducklake:ducklake_reader_secret' AS ducklake_reader;
USE ducklake_reader;

SELECT * FROM some_schema.some_table; -- Works
SELECT * FROM some_schema.another_table; -- Fails
```

The last query will print the following error:

```console
HTTP Error:
HTTP GET error on 'https://ducklake-access-control.s3.amazonaws.com/some_schema/another_table/ducklake-019929c8-c9c9-77d7-91e6-bc3c6dc87605.parquet' (HTTP 403)
```

If we try to create a table, which is just a metadata operation, the error will be different, as it is imposed by a lack of permissions on the PostgreSQL side:

```sql
CREATE TABLE yet_another_table (a INT);
```

```console
TransactionContext Error:
Failed to commit: Failed to commit DuckLake transaction: Failed to write new table to DuckLake: Failed to prepare COPY "COPY "public"."ducklake_table" FROM STDIN (FORMAT BINARY)": ERROR:  permission denied for table ducklake_table
```

### Backup and Recovery {#docs:stable:duckdb:guides:backups_and_recovery}

DuckLake has two components: catalog and storage. The catalog contains all of DuckLake's metadata, while the storage contains all of the data files in Parquet format. The catalog is a [database](#docs:stable:duckdb:usage:choosing_a_catalog_database), while the storage layer can be any [filesystem backend supported by DuckDB](#docs:stable:duckdb:usage:choosing_storage). These two components have different backup strategies, so this document will address them separately.

> In this document, we will focus on disasters caused by human errors or application failures/malfunctions that result in data corruption or loss.

#### Catalog Backup and Recovery {#docs:stable:duckdb:guides:backups_and_recovery::catalog-backup-and-recovery}

Backup and recovery strategies depend on the SQL database you are using as a DuckLake catalog.

> [Compaction](#docs:stable:duckdb:maintenance:merge_adjacent_files) and [cleanup jobs](#docs:stable:duckdb:maintenance:cleanup_of_files) should only be done before manual backups. These operations can re-write and remove data files, effectively changing the file layout for a specific snapshot.

##### DuckDB Catalog {#docs:stable:duckdb:guides:backups_and_recovery::duckdb-catalog}

For DuckDB, the best approach is to perform regular backups of the metadata database. If the original database is corrupted, tampered with, or even deleted, you can recover from this backup.

```sql
-- Backup
ATTACH 'db.duckdb' AS db (READ_ONLY);
ATTACH 'backup.duckdb' AS backup;
COPY FROM DATABASE db TO backup;

-- Recover
ATTACH 'db.duckdb' AS db;
ATTACH 'backup.duckdb' AS backup (READ_ONLY);
COPY FROM DATABASE backup TO db;
ATTACH 'ducklake:db.duckdb' AS my_ducklake;
```

It is very important to note that transactions committed to DuckLake after the metadata backup will not be tracked when recovering. The data from the transactions will exist in the data files, but the backup will point to a previous snapshot. If you are running batch jobs, make sure to always back up after the batch job. If you are regularly micro-batching or streaming data, then schedule periodic jobs to back up your metadata.

> **Tip.** If you want to make a backup with the current timestamp, you need to do this with a specific client. Right now `ATTACH` does not support functions, only strings. This is how it would look in Python:
>
> ```python
> import duckdb
> import datetime
> con = duckdb.connection(f"backup_{datetime.datetime.now().strftime('%Y-%m-%d__%I_%M_%S')}.duckdb")
> ```

##### SQLite Catalog {#docs:stable:duckdb:guides:backups_and_recovery::sqlite-catalog}

For SQLite, the process is exactly the same as with DuckDB and has the same implications.

```sql
-- Backup
ATTACH 'sqlite:db.duckdb' AS db (READ_ONLY);
ATTACH 'sqlite:backup.duckdb' AS backup;
COPY FROM DATABASE db TO backup;

-- Recover
ATTACH 'sqlite:db.duckdb' AS db;
ATTACH 'sqlite:backup.duckdb' AS backup (READ_ONLY);
COPY FROM DATABASE backup TO db;
ATTACH 'ducklake:sqlite:db.duckdb' AS my_ducklake;
```

##### PostgreSQL Catalog {#docs:stable:duckdb:guides:backups_and_recovery::postgresql-catalog}

For PostgreSQL, there are two main approaches to backup and recovery:

- [SQL dump](https://www.postgresql.org/docs/current/backup-dump.html): This approach is similar to the one mentioned for SQLite and DuckDB. This process can happen periodically and can only recover to a particular point in time (i.e., the time of the dump). For DuckLake, this will be a specific snapshot, and transactions after this snapshot will not be recorded.
- [Continuous Archiving and Point-in-Time Recovery (PITR)](https://www.postgresql.org/docs/current/continuous-archiving.html): This is a more complex approach but allows recovery to a specific point in time. For DuckLake, this means you can recover to a specific snapshot without losing any transactions.

Note that the SQL dump approach can also be managed by DuckDB using the [`postgres` extension](https://duckdb.org/docs/current/core_extensions/postgres). In fact, the backup can be a DuckDB file.

> **Warning.** If your Postgres database has indexes, DuckDB will try to copy those over and fail.

```sql
-- Backup
ATTACH 'postgres:connection_string' AS db (READ_ONLY);
ATTACH 'duckdb:backup.duckdb' AS backup;
COPY FROM DATABASE db TO backup;

-- Recover
ATTACH 'postgres:connection_string' AS db;
ATTACH 'duckdb:backup.duckdb' AS backup (READ_ONLY);
COPY FROM DATABASE backup TO db;
ATTACH 'ducklake:postgres:connection_string' AS my_ducklake;
```

> Cloud-hosted PostgreSQL solutions may offer different mechanisms. We encourage you to check what your specific vendor recommends as a strategy for backup and recovery.

#### Storage Backup and Recovery {#docs:stable:duckdb:guides:backups_and_recovery::storage-backup-and-recovery}

Backup and recovery of the data files also depend on the storage you are using. In this document, we will only focus on cloud-based object storage since it is the most common for lakehouse architectures.

##### S3 {#docs:stable:duckdb:guides:backups_and_recovery::s3}

In S3, there are three main mechanisms that AWS offers to back up and/or restore data:

- [Cross-bucket replication](https://docs.aws.amazon.com/AmazonS3/latest/userguide/replication.html)
- [S3 backup service](https://docs.aws.amazon.com/aws-backup/latest/devguide/s3-backups.html)
- [Enable S3 versioning](https://docs.aws.amazon.com/AmazonS3/latest/userguide/Versioning.html)

Both the S3 backup service and S3 object versioning will restore data files in the same bucket. On the other hand, cross-bucket replication will copy the files to a different bucket, and therefore your DuckLake initialization should change:

```sql
-- Before
ATTACH 'ducklake:some.duckdb' AS my_ducklake (DATA_PATH 's3://鉄╫g-bucket鉄�/');

-- After
ATTACH 'ducklake:some.duckdb' AS my_ducklake (DATA_PATH 's3://鉄╮eplication-bucket鉄�/');
```

##### GCS {#docs:stable:duckdb:guides:backups_and_recovery::gcs}

GCS has similar mechanisms to back up and/or restore data:

- [Cross-bucket replication](https://cloud.google.com/storage/docs/using-cross-bucket-replication)
- [Backup and DR service](https://cloud.google.com/backup-disaster-recovery/docs/concepts/backup-dr)
- [Object versioning](https://cloud.google.com/storage/docs/object-versioning) with soft deletes enabled

Regarding cross-bucket replication, repointing to the new bucket will be necessary.

```sql
-- Before
ATTACH 'ducklake:some.duckdb' AS my_ducklake (DATA_PATH 'gs://鉄╫g-bucket鉄�/');

-- After
ATTACH 'ducklake:some.duckdb' AS my_ducklake (DATA_PATH 'gs://鉄╮eplication-bucket鉄�/');
```

### Public DuckLake on Object Storage {#docs:stable:duckdb:guides:public_ducklake_on_object_storage}

This guide explains how to set up a **public read-only DuckLake** on object storage such as Amazon S3, Backblaze B2, Cloudflare R2, [Leafcloud Object Storage](https://szarnyasg.org/posts/ducklake-on-leafcloud/), etc.
Users can query this DuckLake through HTTPS **without authentication**.

> **Warning.** Please check the pricing models of the providers to understand the costs of serving DuckLakes.

> The setup described here is conceptually similar to [Frozen DuckLakes](https://ducklake.select/2025/10/24/frozen-ducklake) but it is simpler to set up.

#### Steps {#docs:stable:duckdb:guides:public_ducklake_on_object_storage::steps}

##### Creating the Bucket {#docs:stable:duckdb:guides:public_ducklake_on_object_storage::creating-the-bucket}

Create a new public bucket:

* [Amazon S3](https://docs.aws.amazon.com/AmazonS3/latest/userguide/create-bucket-overview.html)
* [Backblaze B2](https://www.backblaze.com/docs/cloud-storage-create-and-manage-buckets)
* [Cloudflare R2](https://developers.cloudflare.com/r2/buckets/create-buckets/)

Make sure that the bucket is accessible through the internet. The exact settings for this vary from platform to platform.

##### Creating the DuckLake {#docs:stable:duckdb:guides:public_ducklake_on_object_storage::creating-the-ducklake}

Create a new DuckLake following the [鈥淯sing a Remove Data Path鈥� guide](#docs:stable:duckdb:guides:using_a_remote_data_path) using DuckDB as the catalog database and set the data path to the `https://` URL that serves your bucket.

##### Uploading the DuckLake {#docs:stable:duckdb:guides:public_ducklake_on_object_storage::uploading-the-ducklake}

Upload the `.ducklake` file and its data directory to the bucket. We recommend using [Rclone](https://rclone.org/), which supports all object storage platforms listed above.

```bash
rclone config
```

Create a new Rclone remote (see the details in the foldouts below). In our examples, we will call this `鉄▂our_rclone_remote鉄ー.

<details markdown='1'>
<summary markdown='span'>
Storage configuration in Rclone
</summary>
```text
Option Storage.
Type of storage to configure.
Choose a number from below, or type in your own value.
...
Storage> s3
```
</details>

To upload your data to the R2 bucket, run:

```sql
rclone cp -v 鉄▂our_ducklake_catalog.ducklake鉄� 鉄▂our_rclone_remote鉄�:鉄▂our_bucket_name鉄�/鉄╬ath鉄�/
rclone sync -v 鉄▂our_ducklake_directory鉄� 鉄▂our_rclone_remote鉄�:鉄▂our_bucket_name鉄�/鉄╬ath鉄�/
```

##### Cloudflare: Setting the CORS Policy for Browser Access {#docs:stable:duckdb:guides:public_ducklake_on_object_storage::cloudflare-setting-the-cors-policy-for-browser-access}

If you are using Cloudflare as your storage and try to query the dataset from another website 鈥� such as the [online DuckDB shell](https://shell.duckdb.org/) 鈥�, you will get an error due to the CORS (Cross-Origin Resource Sharing) security mechanism:

```console
IO Error: Failed to attach DuckLake MetaData "__ducklake_metadata_..." at path + "..."
Cannot open database "..." in read-only mode: database does not exist
```

or

```console
Invalid Error: Failed to attach DuckLake MetaData "__ducklake_metadata..." at path + "..."
Opening file '...' failed with error:
NetworkError: Failed to execute 'send' on 'XMLHttpRequest': Failed to load '...'.
```

To allow querying the dataset, add a CORS policy to the Cloudflare configuration of the bucket. How to do this depends on whether you are serving directly from the bucket or through a Cloudflare Worker.

If you are serving through a Cloudflare Worker, [edit the code of the Worker following the 鈥淐ORS header proxy鈥漖(https://developers.cloudflare.com/workers/examples/cors-header-proxy/) and add the following to the JavaScript code of your `fetch` function:

```js
const allowedOrigins = [
  "https://duckdb.org",
  "https://shell.duckdb.org",
];

const origin = request.headers.get("Origin");

let corsOrigin = "";

if (allowedOrigins.includes(origin)) {
  corsOrigin = origin;
}
return new Response(object.body, {
  headers: {
    "Content-Type": contentType,
    "Access-Control-Allow-Origin": corsOrigin,
    "Cache-Control": "public, max-age=3600",
  }
});
```

If you are serving directly from the bucket, navigate to its settings and add the following CORS Policy:

```json
[
  {
    "AllowedOrigins": [
      "https://duckdb.org",
      "https://shell.duckdb.org"
    ],
    "AllowedMethods": [
      "GET"
    ]
  }
]
```

For the complete Cloudflare guide on CORS policies, see the [鈥淐onfigure CORS鈥� page](https://developers.cloudflare.com/r2/buckets/cors/).

### Using a Remote Data Path {#docs:stable:duckdb:guides:using_a_remote_data_path}

This guide shows how to set up and load a DuckLake locally, where the DuckLake will be served on an `https://` endpoint.

> **Tip.** DuckLake currently does not allow you to change the persisted data path in the catalog.
> This is a known limitation that will be lifted in the future, rendering the future version of this guide almost trivial.

In this guide, we assume that we want to create a DuckLake at `https://blobs.duckdb.org/datalake/tpch-sf1/` to serve a read-only copy of the TPC-H SF1 dataset.

#### Initializing the DuckLake {#docs:stable:duckdb:guides:using_a_remote_data_path::initializing-the-ducklake}

We initialize the DataLake using the _remote URL as the data path_ and immediately detach from the DuckLake:

```sql
ATTACH 'tpch-sf1.ducklake' AS tpch_sf1_ducklake (
    TYPE ducklake,
    DATA_PATH 'https://blobs.duckdb.org/datalake/tpch-sf1'
);
DETACH tpch_sf1_ducklake;
```

#### Generating the Data {#docs:stable:duckdb:guides:using_a_remote_data_path::generating-the-data}

Generate the data using the [`tpchgen-cli`](https://github.com/clflushopt/tpchgen-rs/) tool:

```batch
tpchgen-cli --scale-factor 1 --format parquet
```

#### Populating the DuckLake {#docs:stable:duckdb:guides:using_a_remote_data_path::populating-the-ducklake}

Populating the DuckLake requires the following steps:

1. Attach to the DuckLake with a _local data path_ using the `OVERRIDE_DATA_PATH true` flag:

   ```sql
   ATTACH 'tpch-sf1.ducklake' AS tpch_sf1_ducklake (
       TYPE ducklake,
       DATA_PATH 'tpch-sf1',
       OVERRIDE_DATA_PATH true
   );
   USE tpch_sf1_ducklake;
   ```

2. Load the data into the DuckLake:

   ```sql
   CREATE TABLE customer AS FROM 'customer.parquet';
   CREATE TABLE lineitem AS FROM 'lineitem.parquet';
   CREATE TABLE nation AS FROM 'nation.parquet';
   CREATE TABLE orders AS FROM 'orders.parquet';
   CREATE TABLE part AS FROM 'part.parquet';
   CREATE TABLE partsupp AS FROM 'partsupp.parquet';
   CREATE TABLE region AS FROM 'region.parquet';
   CREATE TABLE supplier AS FROM 'supplier.parquet';
   ```

3. Flush the inlined data:

   ```sql
   CALL ducklake_flush_inlined_data('tpch_sf1_ducklake');
   ```

4. Close the DuckDB session with `Ctrl + D` or `.quit`.

#### Uploading the DuckLake {#docs:stable:duckdb:guides:using_a_remote_data_path::uploading-the-ducklake}

Now, we have the `tpch-sf1.ducklake` file and the `tpch-sf1/` directory with the Parquet files:

```batch
tree tpch-sf1
```

```text
tpch-sf1
鈹斺攢鈹€ main
    鈹溾攢鈹€ customer
    鈹偮犅� 鈹斺攢鈹€ ducklake-019a2726-0362-7eae-8a4d-1404ead2c506.parquet
    鈹溾攢鈹€ lineitem
    鈹偮犅� 鈹斺攢鈹€ ducklake-019a2726-03ad-7d79-b28a-6ae3f114fbd3.parquet
    鈹溾攢鈹€ nation
    鈹偮犅� 鈹斺攢鈹€ ducklake-019a2726-055d-7f55-914d-02787bda2eae.parquet
    鈹溾攢鈹€ orders
    鈹偮犅� 鈹斺攢鈹€ ducklake-019a2726-0579-72e2-be9e-0190b3f8f8af.parquet
    鈹溾攢鈹€ part
    鈹偮犅� 鈹斺攢鈹€ ducklake-019a2726-05fd-7d30-8051-a9172d75f815.parquet
    鈹溾攢鈹€ partsupp
    鈹偮犅� 鈹斺攢鈹€ ducklake-019a2726-0645-765b-9456-6ba632d8288b.parquet
    鈹溾攢鈹€ region
    鈹偮犅� 鈹斺攢鈹€ ducklake-019a2726-06a4-7b3a-9788-8f32006be0ad.parquet
    鈹斺攢鈹€ supplier
        鈹斺攢鈹€ ducklake-019a2726-06bb-72f4-97b3-131d7186787e.parquet
```

We upload both of these to `https://blobs.duckdb.org/datalake/`.
This particular URL is served by Cloudflare and is based on the content of a public Cloudflare R2 bucket 鈥� but DuckLake works with any `http(s)://` URL.

#### Using the DuckLake {#docs:stable:duckdb:guides:using_a_remote_data_path::using-the-ducklake}

You can now attach to the remote DuckLake as follows:

```sql
ATTACH 'https://blobs.duckdb.org/datalake/tpch-sf1.ducklake' AS tpch_sf1_ducklake (TYPE ducklake);
USE tpch_sf1_ducklake;
```

Now you can use it like any other DuckDB database or DuckLake:

```sql
SELECT count(*) FROM lineitem;
```

```text
鈹屸攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹�
鈹�  count_star()  鈹�
鈹�     int64      鈹�
鈹溾攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹�
鈹�    6001215     鈹�
鈹� (6.00 million) 鈹�
鈹斺攢鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹�
```

### Troubleshooting {#docs:stable:duckdb:guides:troubleshooting}

This guide explains how to troubleshoot certain issues with DuckLake.
Besides the issues described on this page, see also the [鈥淯nsupported Features鈥� page](#docs:stable:duckdb:unsupported_features).

#### Connecting to an Older DuckLake {#docs:stable:duckdb:guides:troubleshooting::connecting-to-an-older-ducklake}

If you try to connect to a DuckLake created by an older version of the `ducklake` extension, you get the following error message:

```console
Invalid Input Error:
DuckLake catalog version mismatch: catalog version is 0.3, but the extension requires version 1.0.
To automatically migrate, set AUTOMATIC_MIGRATION to TRUE when attaching.
```

To work around this, add the `AUTOMATIC_MIGRATION` flag to the `ATTACH` command:

```sql
ATTACH '...' (AUTOMATIC_MIGRATION);
```

#### Connecting to a Read-Only DuckLake {#docs:stable:duckdb:guides:troubleshooting::connecting-to-a-read-only-ducklake}

If you try to connect to a read-only DuckLake, you get the following error message:

```console
Invalid Input Error:
Failed to migrate DuckLake from v0.3 to v1.0:
Cannot execute statement of type "CREATE" on database "__ducklake_metadata_sf1" which is attached in read-only mode!
```

It is not possible to connect to a read-only DuckLake with a pre-release version (v0.x).
To migrate from such a DuckLake, use an older version of the `ducklake` extension and migrate into an intermediate format.
For example, copy the data into a DuckLake that you have write access to, then connect to that DuckLake using the newer `ducklake` extension.

## Unsupported Features {#docs:stable:duckdb:unsupported_features}

This page describes what is supported in DuckDB and DuckLake in relation to DuckDB standalone (i.e., `:memory:` or DuckDB file modes). We can make a distinction between:

- What is **currently** not supported by the DuckLake specification. These are features that are supported by DuckDB when using DuckDB's native database format but will not work with a DuckLake backend since the specification does not support them.

- What is **currently** not supported by the `ducklake` DuckDB extension. These are features that are supported by the DuckLake specification but are not (yet) implemented in the DuckDB extension.

#### Unsupported by the DuckLake Specification {#docs:stable:duckdb:unsupported_features::unsupported-by-the-ducklake-specification}

Within this group, we are going to make a distinction between what is not supported now but is likely to be supported in the future and what is not supported and is unlikely to be supported.

##### Likely to Be Supported in the Future {#docs:stable:duckdb:unsupported_features::likely-to-be-supported-in-the-future}

The following features are currently not supported in DuckLake but are likely to be supported in future versions:

- [User defined types](https://duckdb.org/docs/current/sql/statements/create_type).

- Fixed-size arrays, i.e., [`ARRAY` type](https://duckdb.org/docs/current/sql/data_types/array)

- [`ENUM` type](https://duckdb.org/docs/current/sql/data_types/enum)

- [`CHECK` constraints](https://duckdb.org/docs/current/sql/constraints#check-constraint) (not to be confused with Primary or Foreign Key constraints)

- Default values that are not literals. See the following example:

  ```sql
  -- This is allowed
  CREATE TABLE t1 (id INTEGER, d DATE DEFAULT '2025-08-08');

  -- This is not allowed
  CREATE TABLE t1 (id INTEGER, d DATE DEFAULT now());
  ```

- Dropping dependencies, such as views, when calling `DROP ... CASCADE`. Note that this is also a [DuckDB limitation](https://duckdb.org/docs/current/sql/statements/drop#dependencies-on-views).

- [Generated columns](https://duckdb.org/docs/current/sql/statements/create_table#generated-columns)

##### Unlikely to Be Supported in the Future {#docs:stable:duckdb:unsupported_features::unlikely-to-be-supported-in-the-future}

The following features are currently not supported in DuckLake and are unlikely to be supported in future versions:

- [Indexes](https://duckdb.org/docs/current/sql/indexes)

- [Primary key or enforced unique constraints](https://duckdb.org/docs/current/sql/constraints#primary-key-and-unique-constraint) and [foreign key constraints](https://duckdb.org/docs/current/sql/constraints#foreign-keys) are unlikely to be supported as these are prohibitively expensive to enforce in data lake setups. We may consider supporting unenforced primary keys, similar to [BigQuery's implementation](https://cloud.google.com/bigquery/docs/primary-foreign-keys).

- Upserting is only supported via the [`MERGE INTO`](#docs:stable:duckdb:usage:upserting) syntax since primary keys are not supported in DuckLake.

- [Sequences](https://duckdb.org/docs/current/sql/statements/create_sequence)

- [`VARINT` type](https://duckdb.org/docs/current/sql/data_types/numeric#variable-integer)

- [`BITSTRING` type](https://duckdb.org/docs/current/sql/data_types/bitstring)

- [`UNION` type](https://duckdb.org/docs/current/sql/data_types/union)

# Acknowledgments

This document is built with [Pandoc](https://pandoc.org/) using the [Eisvogel template](https://github.com/Wandmalfarbe/pandoc-latex-template). The scripts to build the document are available in the [`ducklake-web` repository](https://github.com/duckdb/ducklake-web/tree/main/single-file-document).

The emojis used in this document are provided by [Twemoji](https://twemoji.twitter.com/) under the [CC-BY 4.0 license](https://creativecommons.org/licenses/by/4.0/).

The syntax highlighter uses the [Bluloco Light theme](https://github.com/uloco/theme-bluloco-light) by Umut Topuzo臒lu.