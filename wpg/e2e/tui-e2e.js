// Browser-backend e2e for boru:tui in the wasm playground (the RFC §9
// playground tier): load the bundled page, run a counter app, drive it
// with real keystrokes, quit, and assert the final state lands back in
// the REPL. Opt-in (needs a built bundle and a chromium):
//
//   cd wpg && make wasm && make e2e
//
// Requirements: node with playwright resolvable (PLAYWRIGHT_MODULE
// overrides the module path), and a chromium binary (PLAYWRIGHT_CHROME
// overrides; defaults try the playwright-managed install).
const pwModule = process.env.PLAYWRIGHT_MODULE || 'playwright';
const { chromium } = require(pwModule);

const bundle = process.env.BORU_PLAYGROUND ||
  require('path').resolve(__dirname, '../../docs/index.html');

(async () => {
  const launch = {};
  if (process.env.PLAYWRIGHT_CHROME) launch.executablePath = process.env.PLAYWRIGHT_CHROME;
  const browser = await chromium.launch(launch);
  const page = await browser.newPage();
  page.on('console', m => { if (m.type() === 'error') console.log('PAGE-ERR:', m.text()); });
  await page.goto('file://' + bundle);

  await page.waitForFunction(() => typeof boruEvalAsync === 'function', { timeout: 60000 });
  console.log('engine ready');

  const app = `import "boru:tui"
def final (Tui.run {init: 0  title: "counter"
  update: ([s:Any e:Map] => [ if (e.key eq "up") [ (s add 1) ] [ if (e.key eq "q") [ Tui.quit s ] [ s ] ] ])
  view: ([s:Any] => [ Tui.text (convert String s) ])})
join "" ["final=" (convert String final)]`;

  await page.fill('#input', app);
  await page.click('#run-btn');

  // the terminal panel appears with the init frame and the app title
  await page.waitForSelector('#tui-view:not([hidden])', { timeout: 20000 });
  await page.waitForFunction(() => document.getElementById('tui-screen').textContent.includes('0'), { timeout: 10000 });
  const title = await page.textContent('#tui-title');
  if (!title.includes('counter')) throw new Error('title not set: ' + title);
  console.log('panel open, title =', JSON.stringify(title.trim()));

  // two increments, watched on the live screen
  await page.focus('#tui-screen');
  await page.keyboard.press('ArrowUp');
  await page.waitForFunction(() => document.getElementById('tui-screen').textContent.includes('1'), { timeout: 10000 });
  await page.keyboard.press('ArrowUp');
  await page.waitForFunction(() => document.getElementById('tui-screen').textContent.includes('2'), { timeout: 10000 });
  console.log('screen counted 0 -> 1 -> 2');

  // q quits: the panel hides and the final state lands in the REPL
  await page.keyboard.press('q');
  await page.waitForFunction(() => document.getElementById('tui-view').hidden, { timeout: 10000 });
  await page.waitForFunction(() => document.getElementById('output').textContent.includes('final=2'), { timeout: 10000 });
  console.log('quit returned final=2 to the REPL');

  await browser.close();
  console.log('E2E PASS');
})().catch(e => { console.error('E2E FAIL:', e.message); process.exit(1); });
