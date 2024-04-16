<script>
  import { onMount } from "svelte";

  const apiURL = import.meta.env.VITE_API_URL;
  let selectedDB =
    "367fb8f59f92e9b1cb0820f692e4c0d426a886043dd6ae1eac2fbc6cacf6cada";
  let selectedTab = null;
  let tables = [];
  let tablesLoading = true;

  let rows = [];
  let rowsLoading = false;

  let columns = [];
  let columnsLoading = false;

  let addRecordMode = false;
  let rowLimit = 20;
  let rowOffset = 0;

  let inputSQL = "";
  let timecost = 0;

  onMount(async () => {
    fetchDBList();
  });

  function fetchDBList() {
    const params = new URLSearchParams(window.location.search);
    rowLimit = params.get("limit") || 20;
    rowOffset = params.get("offset") || 0;
    fetch(`${apiURL}/api/v1/${selectedDB}/tables`)
      .then((response) => response.json())
      .then(({ data }) => {
        tables = data.values.map((row) => {
          return {
            name: row[0],
          };
        });
        selectedTab = data.values[0][0];
        handleTableTabClick(selectedTab);
        tablesLoading = false;
      })
      .catch((error) => {
        console.log(error);
        return [];
      });
  }

  function handleTableTabClick(tableName) {
    rowOffset = 0;

    rowsLoading = true;
    columnsLoading = true;
    selectedTab = tableName;
    rows = [];
    columns = [];

    addRecordMode = false;

    fetch(
      `${apiURL}/api/v1/${selectedDB}/tables/${tableName}/rows?limit=${rowLimit}&offset=${rowOffset}`,
    )
      .then((response) => response.json())
      .then(({ data }) => {
        columns = data.columns.map((row) => {
          return {
            name: row,
          };
        });
        columnsLoading = false;

        rows = data.values.map((row) => {
          let fields = {};
          for (const key in row) {
            fields[key] = {
              value: row[key],
              editable: false,
            };
          }
          return { fields };
        });
        rowsLoading = false;
        timecost = data.cost;
      })
      .catch((error) => {
        console.log(error);
        return [];
      });
  }

  function handleDBSelect(event) {
    selectedDB = event.target.value;
    fetchDBList();
  }

  function handleExecuteSQL() {
    fetch(`${apiURL}/api/v1/${selectedDB}/query`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        list: [
          {
            cmd: inputSQL,
          },
        ],
      }),
    })
      .then((response) => response.json())
      .then(({ data }) => {
        columns = [];
        rows = [];
        selectedTab = "";

        let result0 = data[0] || {};
        columns = result0.columns.map((row) => {
          return {
            name: row,
          };
        });
        rows = result0.values.map((row) => {
          let fields = {};
          for (const key in row) {
            fields[key] = {
              value: row[key],
              editable: false,
            };
          }
          return { fields };
        });
        timecost = result0.cost;
      })
      .catch((error) => {
        console.log(error);
        return [];
      });
  }

  function handleLoadMoreRecord() {
    rowOffset = rowOffset + 1;

    fetch(
      `${apiURL}/api/v1/${selectedDB}/tables/${selectedTab}/rows?limit=${rowLimit}&offset=${rowOffset}`,
    )
      .then((response) => response.json())
      .then(({ data }) => {
        let newrows = data.values.map((row) => {
          let fields = {};
          for (const key in row) {
            fields[key] = {
              value: row[key],
              editable: false,
            };
          }
          return { fields };
        });
        rows = rows.concat(newrows);
        timecost = data.cost;
      })
      .catch((error) => {
        console.log(error);
        return [];
      });
  }
</script>

<div class="wrapper">
  {#if tablesLoading}
    <p>Loading...</p>
  {:else}
    <ul class="tables">
      <li>
        <button class="tami"> LessDB </button>
      </li>
      {#each tables as table}
        <li>
          <button
            class:selected={table.name === selectedTab}
            on:click={handleTableTabClick(table.name)}
          >
            {table.name}
          </button>
        </li>
      {/each}
    </ul>
    <div class="table-wrapper">
      <div class="flex-container">
        <p>DBName :</p>
        <select class="select" on:change={handleDBSelect}>
          <option
            value="367fb8f59f92e9b1cb0820f692e4c0d426a886043dd6ae1eac2fbc6cacf6cada"
            >pub_demo1</option
          >
        </select>
        <input
          class="text"
          type="text"
          placeholder="input sql to execute"
          bind:value={inputSQL}
        />
        <button class="button primary" on:click={handleExecuteSQL}>
          Execute SQL
        </button>
      </div>
      <table cellspacing="0">
        <thead>
          <tr>
            <th class="select">
              <input type="checkbox" />
            </th>
            {#each columns as column}
              <th>{column.name}</th>
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each rows as row}
            <tr class:new={row.new} class="table-row">
              <td class="select">
                <input type="checkbox" />
              </td>
              {#each Object.keys(row.fields) as key}
                <td
                  class:editing={row.fields[key].editable}
                  on:dblclick|stopPropagation={() => {
                    row.fields[key].editable = true;
                  }}
                >
                  {#if !row.fields[key].editable}
                    <div class="cell">
                      {row.fields[key].value}
                    </div>
                  {:else}
                    <input
                      class="cell"
                      bind:this={row.fields[key].input}
                      bind:value={row.fields[key].value}
                      on:blur={() => {
                        if (row.new) return;
                        row.fields[key].editable = false;
                      }}
                      placeholder={row.fields[key].placeholder}
                    />
                  {/if}
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <button class="button primary" on:click={handleLoadMoreRecord}>
      Load More (time cost: {timecost} ms)
    </button>
  {/if}
</div>

<style>
  ul.tables {
    margin: 0;
    list-style: none;
    padding: 0;
    display: flex;
    background-color: var(--background-1);
    border-bottom: 1px solid var(--divider);
    max-width: 150vw;
  }

  ul.tables li button {
    text-transform: capitalize;
    font-weight: bold;
    border: none;
    background: none;
    color: var(--text-1);
    padding: 0 1.5rem;
    height: 50px;
    border-right: 1px solid var(--divider);
    transition: 0.3s all;
  }

  ul.tables li button:hover {
    background: var(--background-3);
  }

  .tami {
    background-color: var(--primary-0) !important;
    font-size: 22px;
  }

  ul.tables li button.selected {
    background: var(--background-2);
  }

  thead {
    background-color: var(--background-2);
  }

  td,
  th {
    text-align: left;
    height: 35px;
    margin: 0;
    font-size: 18px;
    border-right: 1px solid var(--divider);
    border-bottom: 1px solid var(--divider);
    padding: 0 1rem;
  }
  th {
    padding: 0 1rem;
  }
  td .cell {
    background-color: transparent;
    border: none !important;
    padding: 0 !important;
    color: var(--text-1);
    max-width: 300px;
    margin: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  td input.cell {
    height: 100%;
  }
  th {
    border-top: 1px solid var(--divider);
  }
  .new {
    background-color: var(--warning-darker-5);
  }
  .new:hover {
    background-color: var(--warning-darker-5) !important;
  }
  th.select,
  td.select {
    min-width: unset !important;
  }
  .table-row:hover {
    background: var(--background-2);
    transition: 0.3s all;
  }
  .table-wrapper {
    position: relative;
  }
  table {
    z-index: 100;
    position: relative;
  }
  .button {
    border: none;
    background: var(--background-2);
    font-weight: bold;
    width: 400px;
    height: 30px;
    color: var(--text-1);
    border-radius: 2px;
    margin: 1rem;
  }
  .button:hover {
    background: var(--background-3);
  }
  .button.primary {
    background: var(--primary-0);
  }
  .button.primary:hover {
    background: var(--primary-1);
  }
  .flex-container {
    display: flex;
    margin: 0px;
  }
  .flex-container p {
    display: flex;
    font-weight: bold;
    justify-content: center;
    align-items: center;
  }
  .flex-container .select {
    display: flex;
    border: none;
    background: var(--background-2);
    font-weight: bold;
    width: 200px;
    color: var(--text-1);
    border-radius: 2px;
    margin: 10px;
  }
  .flex-container .text {
    display: flex;
    border: none;
    background: var(--background-2);
    font-weight: bold;
    min-width: 600px;
    color: var(--text-1);
    border-radius: 2px;
    margin: 10px;
  }
  .flex-container button {
    border: none;
    background: var(--background-2);
    font-weight: bold;
    width: 200px;
    color: var(--text-1);
    border-radius: 2px;
  }
</style>
