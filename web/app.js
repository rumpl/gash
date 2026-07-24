const status = document.querySelector("#status");
const script = document.querySelector("#script");
const output = document.querySelector("#output");
const runButton = document.querySelector("#run");
const resetButton = document.querySelector("#reset");

async function loadGash() {
  const go = new Go();
  const response = await fetch("gash.wasm");
  if (!response.ok) throw new Error(`fetch gash.wasm: ${response.status}`);
  const bytes = await response.arrayBuffer();
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  go.run(instance).catch(error => {
    status.textContent = "Runtime stopped";
    console.error(error);
  });
  while (!globalThis.gash) await new Promise(resolve => setTimeout(resolve, 0));
}

async function run() {
  runButton.disabled = true;
  output.className = "";
  output.textContent = "Running…";
  try {
    const result = await gash.exec(script.value);
    output.textContent = result.stdout + result.stderr || "(no output)";
    output.classList.toggle("error", result.exitCode !== 0);
    status.textContent = `Exit ${result.exitCode}`;
  } catch (error) {
    output.textContent = String(error);
    output.className = "error";
    status.textContent = "Error";
  } finally {
    runButton.disabled = false;
  }
}

runButton.addEventListener("click", run);
resetButton.addEventListener("click", () => {
  const error = gash.reset();
  status.textContent = error ? `Reset failed: ${error}` : "Filesystem reset";
});
script.addEventListener("keydown", event => {
  if ((event.ctrlKey || event.metaKey) && event.key === "Enter") run();
});

loadGash().then(() => {
  status.textContent = "Ready";
  output.textContent = "Ready. Run the example script.";
  runButton.disabled = false;
  resetButton.disabled = false;
}).catch(error => {
  status.textContent = "Failed to load";
  output.textContent = `${error}\n\nBuild the demo with “make wasm” and serve the repository over HTTP.`;
  output.className = "error";
});
