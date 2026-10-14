/* Bundled Swagger UI; no remote validator or sample API. */
"use strict";
(async function () {
  const actions = document.getElementById("enable-actions");
  const response = await fetch("/openapi.json", {cache:"no-store"});
  if (!response.ok) throw new Error("Contract could not be loaded.");
  const spec = await response.json();
  spec.servers = [{url:window.location.origin, description:"This Companion"}];
  const escapeSegment = segment => Array.from(segment).map(char =>
    "^$.|?*+()[]{}".includes(char) ? String.fromCharCode(92) + char : char).join("");
  const operations = Object.entries(spec.paths).flatMap(([path, item]) =>
    Object.entries(item).filter(([method]) => ["get","post"].includes(method)).map(([method, operation]) => ({
      method, path, operation,
      match:new RegExp("^" + path.split("/").map(segment => segment.startsWith("{") ? "[^/]+" : escapeSegment(segment)).join("/") + "$"),
      specificity:path.split("/").filter(segment => !segment.startsWith("{")).length,
    }))
  ).sort((a,b) => b.specificity-a.specificity);
  const config = {
    spec, dom_id:"#swagger-ui", deepLinking:true, displayOperationId:true,
    showExtensions:true, validatorUrl:null, queryConfigEnabled:false,
    supportedSubmitMethods:["get"], tryItOutEnabled:false,
    requestInterceptor(request) {
      const target = new URL(request.url, window.location.origin);
      if (target.origin !== window.location.origin) throw new Error("Explorer requests must use this Companion origin.");
      const found = operations.find(item => item.method === String(request.method).toLowerCase() && item.match.test(target.pathname));
      if (!found || found.operation["x-explorer-execution"] === "disabled")
        throw new Error("Streaming and file/media operations are documentation-only.");
      if (found.operation["x-explorer-execution"] !== "read") {
        if (!actions.checked) throw new Error("Enable actions above before executing this operation.");
        if (!window.confirm("Execute " + found.operation.operationId + "? This can change Companion state or files."))
          throw new Error("Execution cancelled.");
      }
      return request;
    },
  };
  const render = () => {
    window.ui = SwaggerUIBundle({...config, supportedSubmitMethods:actions.checked ? ["get","post"] : ["get"]});
  };
  render();
  actions.addEventListener("change", render);
})().catch(error => { document.getElementById("error").textContent = error.message; });
