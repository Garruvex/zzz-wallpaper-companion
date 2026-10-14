// Run through TestDocsBrowser with COMPANION_DOCS_BROWSER=1.
const assert = require("node:assert/strict");
const {chromium} = require(process.env.SMOKE_PLAYWRIGHT || "playwright");
(async () => {
 const browser = await chromium.launch({headless:true, executablePath:process.env.SMOKE_BROWSER});
 try {
  const page = await browser.newPage({viewport:{width:1280,height:900}});
  const errors=[], remote=[];
  page.on("pageerror",error=>errors.push(error.message));
  const origin = process.argv[2];
  await page.route("**/*",route=>{
   if (new URL(route.request().url()).origin===origin) return route.continue();
   remote.push(route.request().url());
   return route.abort();
  });
  await page.goto(origin+"/docs");
  await page.waitForSelector(".opblock");
  assert.equal(await page.locator(".opblock").count(),29);
  assert.deepEqual(await page.evaluate(()=>window.ui.getConfigs().supportedSubmitMethods),["get"]);
  assert.equal(await page.evaluate(()=>window.ui.getConfigs().spec.servers[0].url),origin);
  assert.equal(await page.evaluate(()=>window.ui.getConfigs().validatorUrl),null);
  const health=page.locator(".opblock").filter({hasText:"health.get"});
  await health.locator(".opblock-summary").click();
  await health.getByRole("button",{name:"Try it out",exact:true}).click();
  const healthResponse = page.waitForResponse(response=>response.url()===origin+"/api/health");
  await health.getByRole("button",{name:"Execute",exact:true}).click();
  assert.equal((await healthResponse).status(),200);
  await page.waitForFunction(()=>document.querySelector(".opblock .responses-inner")?.textContent.includes("zzz-wallpaper-companion"));
  assert.match(await health.locator(".responses-inner").innerText(),/zzz-wallpaper-companion/);
  var guards=await page.evaluate(origin=>{
   const intercept=window.ui.getConfigs().requestInterceptor;
   const blocked=(url,method)=>{try{intercept({url,method});return false}catch{return true}};
   return {
    wrongOrigin:blocked("http://example.com/api/health","GET"),
    write:blocked(origin+"/api/settings","POST"),
    sideEffectGet:blocked(origin+"/api/v1/carts","GET"),
    media:blocked(origin+"/api/youtube/transcode?id=abcdef&session=abcdefghijklmnop","GET"),
    sse:blocked(origin+"/api/v1/input","GET"),
    file:blocked(origin+"/api/v1/carts/a/save","GET"),
    details:!blocked(origin+"/api/v1/carts/a/details","GET"),
   };
  },origin);
  assert.ok(Object.values(guards).every(Boolean),JSON.stringify(guards));
  await page.locator("#enable-actions").check();
  await page.waitForFunction(()=>window.ui.getConfigs().supportedSubmitMethods.includes("post"));
  var confirmCount=0;
  page.on("dialog",async dialog=>{confirmCount++;await dialog.dismiss()});
  const cancelled=await page.evaluate(origin=>{
   try {window.ui.getConfigs().requestInterceptor({url:origin+"/api/settings",method:"POST"});return false}
   catch(error){return /cancelled/.test(error.message)}
  },origin);
  assert.equal(cancelled,true);assert.equal(confirmCount,1);
  await page.locator("#enable-actions").uncheck();
  assert.deepEqual(await page.evaluate(()=>window.ui.getConfigs().supportedSubmitMethods),["get"]);
  await health.getByRole("button",{name:"Try it out",exact:true}).waitFor();
  if(process.env.DOCS_SCREENSHOT) await page.screenshot({path:process.env.DOCS_SCREENSHOT});
  assert.deepEqual(errors,[]); assert.deepEqual(remote,[]);
  console.log("29 operations; offline assets, nondefault origin, health execution and action/media guards passed.");
 } finally {await browser.close();}
})().catch(error=>{console.error(error);process.exitCode=1});
