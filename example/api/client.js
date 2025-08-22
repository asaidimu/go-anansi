import APIClient from './sdk.js'; // assumes you renamed the minimal sdk to sdk.js

let currentCollection = null;
let currentDocuments = [];
let schemaMap = {};
const client = new APIClient("http://localhost:8081");

let activeTab = 'data';

window.addEventListener("DOMContentLoaded", () => {
  loadCollections();
});

async function loadCollections() {
  const ul = document.getElementById("collectionsList");
  ul.innerHTML = "<li class='text-gray-500'>Loading collections...</li>";
  try {
    const res = await client.listCollections();
    const collections = res.collections || [];
    if (collections.length === 0)
      return (ul.innerHTML = "<li class='text-gray-400'>No collections</li>");

    ul.innerHTML = "";
    collections.forEach((col, idx) => {
      const li = document.createElement("li");
      li.textContent = col;
      li.className = "px-3 py-2 rounded cursor-pointer hover:bg-gray-100";
      li.onclick = () => selectCollection(col);
      ul.appendChild(li);
      if (idx === 0) selectCollection(col); // auto select first
    });

  } catch (err) {
    ul.innerHTML = `<li class=\"text-red-500\">${err.message}</li>`;
  }
}

window.selectCollection = async function (name) {
  currentCollection = name;
  document.querySelectorAll("#collectionsList li").forEach(li => {
    li.classList.remove("bg-gray-200", "font-semibold");
    if (li.textContent === name) li.classList.add("bg-gray-200", "font-semibold");
  });
  document.getElementById("contentTitle").textContent = name;
  await loadSchema();
  await loadDocuments();
};

async function loadSchema() {
  const data = await client.getCollectionSchema(currentCollection);
  schemaMap = data.fields || {};
  if (activeTab === 'schema') renderSchemaView();
}

async function loadDocuments() {
  let query = {};
  try {
    const input = document.getElementById("queryInput");
    if (input && input.value) query = JSON.parse(input.value);
  } catch (_) {}
  const res = await client.readDocuments(currentCollection, { query });
  const docs = res.count === 1 ? [res.data] : res.data;
  currentDocuments = docs;
  if (activeTab === 'data') renderTable(docs);
}

function renderTable(documents) {
  const fields = Object.values(schemaMap)
    .filter(f => f.name && f.type && f.name !== '_metadata_')
    .map(f => f.name);
  const container = document.getElementById('documentsContent');
  if (!documents.length || !fields.length) {
    container.innerHTML = '<p class="text-center text-gray-400 py-20">No documents found</p>';
    return;
  }

  const table = document.createElement('table');
  table.className = 'min-w-full text-sm border border-gray-200';
  const thead = document.createElement('thead');
  thead.innerHTML = `<tr class="bg-gray-100">
    ${fields.map(f => `<th class="px-4 py-2 text-left border-b border-gray-200">${f}</th>`).join('')}
    <th class="px-4 py-2 border-b border-gray-200">Actions</th>
  </tr>`;

  const tbody = document.createElement('tbody');
  documents.forEach((doc, i) => {
    const row = document.createElement('tr');
    row.className = i % 2 === 0 ? 'bg-white' : 'bg-gray-50';
    row.innerHTML = fields.map(f => `<td class="px-4 py-2 border-b border-gray-100 font-mono">${JSON.stringify(doc[f] ?? '')}</td>`).join('');
    row.innerHTML += `<td class="px-4 py-2 border-b border-gray-100">
      <button class="text-blue-600 hover:underline" onclick="alert('Edit not implemented')">Edit</button>
      <button class="text-red-600 hover:underline ml-2" onclick="alert('Delete not implemented')">Delete</button>
    </td>`;
    tbody.appendChild(row);
  });
  table.append(thead, tbody);
  container.innerHTML = '';
  container.appendChild(table);
}

function renderSchemaView() {
  const container = document.getElementById('documentsContent');
  const fields = Object.values(schemaMap).filter(f => f.name && f.type);
  if (!fields.length) {
    container.innerHTML = '<p class="text-center text-gray-400 py-20">No schema fields</p>';
    return;
  }
  const rows = fields.map(f => `<tr>
    <td class="border px-4 py-2 font-mono">${f.name}</td>
    <td class="border px-4 py-2">${f.type}</td>
    <td class="border px-4 py-2">${f.required ? '✅' : ''}</td>
    <td class="border px-4 py-2">${f.unique ? '🔒' : ''}</td>
  </tr>`).join('');
  container.innerHTML = `
    <table class="table-auto border-collapse border border-gray-200 w-full text-sm">
      <thead class="bg-gray-100">
        <tr>
          <th class="border px-4 py-2 text-left">Field</th>
          <th class="border px-4 py-2 text-left">Type</th>
          <th class="border px-4 py-2 text-left">Required</th>
          <th class="border px-4 py-2 text-left">Unique</th>
        </tr>
      </thead>
      <tbody>${rows}</tbody>
    </table>`;
}

window.switchTab = function (tab) {
  activeTab = tab;
  document.querySelectorAll('.tab').forEach(t => {
    t.classList.remove('text-indigo-600', 'border-indigo-600', 'border-b-2');
    t.classList.add('text-gray-600');
  });
  event.target.classList.remove('text-gray-600');
  event.target.classList.add('text-indigo-600', 'border-indigo-600', 'border-b-2');
  document.getElementById("queryContainer").style.display = tab === 'data' ? 'block' : 'none';
  if (tab === 'data') renderTable(currentDocuments);
  else renderSchemaView();
};

window.openSchemaEditor = function () {
  document.getElementById("schemaModal").classList.remove("hidden");
};

window.closeSchemaEditor = function () {
  document.getElementById("schemaModal").classList.add("hidden");
};

window.submitSchema = async function () {
  const text = document.getElementById("schemaInput").value.trim();
  if (!text) return;
  try {
    const parsed = JSON.parse(text);
    await client.createCollection(parsed);
    closeSchemaEditor();
    loadCollections();
  } catch (err) {
    alert("Failed to create collection: " + err.message);
  }
};

