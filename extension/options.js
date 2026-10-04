const api = globalThis.browser ?? globalThis.chrome;
const input = document.getElementById('port');
const status = document.getElementById('status');

api.storage.sync.get({ port: 8787 }).then(({ port }) => {
  input.value = port;
});

document.getElementById('form').addEventListener('submit', async (e) => {
  e.preventDefault();
  const port = Number(input.value);
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    status.textContent = 'Enter a port from 1 to 65535.';
    return;
  }
  await api.storage.sync.set({ port });
  status.textContent = `Saved. The extension now sends links to http://127.0.0.1:${port}/.`;
});
