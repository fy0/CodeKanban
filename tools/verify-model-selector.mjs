/** Build ui/dist and set CK_TEST_BINARY before running this browser regression.
 * Uses a disposable backend/database/profile and fixture model metadata; sends no model messages.
 */
import assert from "node:assert/strict";
import { mkdirSync } from "node:fs";
import path from "node:path";
import { createTestEnvironment } from "./ui-test-environment.mjs";
import { launchBrowser, delay } from "./ui-browser.mjs";

const codexModels = ["gpt-6.1-sol", "gpt-6-sol", "gpt-6-luna", "gpt-5.4"];
const names = ["first", "second", "third", "fourth"];
const config = {
  contextWindowTokens: 480000,
  compactLimitTokens: 450000,
  source: "model_catalog",
  hasCodex: true,
  hasClaudeCode: true,
  hasPi: true,
  hasDevin: true,
  supportsWebSession: true,
  supportsPiWebSession: true,
  piRpcCompatible: true,
  supportsDevinWebSession: true,
  supportsGoalMode: false,
  webSessionMinCodexVersion: "0.1.0",
  goalModeMinCodexVersion: "0.1.0",
  models: codexModels.map((model) => ({
    model,
    displayName: model,
    defaultReasoningEffort: "high",
    supportedReasoningEfforts: ["low", "high"],
  })),
  piModels: [
    ...names,
    ...Array.from({ length: 400 }, (_, i) => `extra-${i}`),
  ].map((id) => ({
    provider: id === "fourth" ? "other-fixture" : "fixture",
    id,
    name: `Fixture ${id}`,
    reasoning: true,
    input: ["text"],
    contextWindow: 480000,
    maxTokens: 64000,
  })),
  devinModels: names.map((model) => ({
    model,
    displayName: `Fixture ${model}`,
    family: model,
    description: "Fixture model",
    defaultReasoningEffort: "high",
    supportedReasoningEfforts: ["high"],
  })),
};

let env;
let browser;
const errors = [];
const passed = [];
try {
  env = await createTestEnvironment({ sessionCount: 1 });
  browser = await launchBrowser();
  const page = await browser.cdp.page();
  browser.cdp.on("Runtime.exceptionThrown", (event, sid) => {
    if (sid === page.sessionId) {
      errors.push(
        event.exceptionDetails.exception?.description ||
          event.exceptionDetails.text,
      );
    }
  });
  await page.viewport(1440, 900);
  await page.command("Page.addScriptToEvaluateOnNewDocument", {
    source: `(() => {
      localStorage.setItem('codekanban-web-session-pi-frequent-models', JSON.stringify(['fixture/first']));
      localStorage.setItem('codekanban-web-session-codex-recent-models', JSON.stringify(['__codex_selection__:' + JSON.stringify(['gpt-6.1-sol', 'high'])]));
      localStorage.setItem('codekanban-web-session-devin-frequent-models', JSON.stringify(['first']));
      const fetchOriginal = window.fetch;
      window.fetch = function(input, init) {
        const url = String(input instanceof Request ? input.url : input);
        if (url.includes('/web-sessions/runtime-config')) {
          return Promise.resolve(new Response(${JSON.stringify(JSON.stringify({ item: config }))}, {headers:{'Content-Type':'application/json'}}));
        }
        return fetchOriginal.call(this, input, init);
      };
    })();`,
  });
  const menuVisible =
    "document.querySelector('.web-session-model-select-menu')?.checkVisibility()";
  const searchInput =
    ".web-session-model-select-menu .pi-model-search-header input";
  const normalize = (text) => text.replace(/[^a-z0-9]/gi, "").toLowerCase();
  let activeSessionId;
  async function openMenu() {
    await page.click(".model-select");
    await page.wait(menuVisible);
    await delay(250);
  }
  async function selectOption(
    agent,
    label,
    { query, composing = false, recent = false } = {},
  ) {
    await openMenu();
    if (query) {
      await page.click(searchInput);
      await page.command("Input.insertText", { text: query });
      await delay(100);
    }
    if (composing) {
      // A row can be selected before compositionend arrives, then move into Recently Used.
      await page.evaluate(
        `document.querySelector(${JSON.stringify(searchInput)}).dispatchEvent(new CompositionEvent('compositionstart', {bubbles:true}))`,
      );
    }
    const point = await page.evaluate(`(() => {
      const option = [...document.querySelectorAll('.web-session-model-select-menu .n-base-select-option')].find(e =>
        e.innerText.replace(/[^a-z0-9]/gi, '').toLowerCase().includes(${JSON.stringify(normalize(label))}) &&
        (!${recent} || e.dataset.modelRecent === 'true')
      );
      if (!option) throw Error('Missing option: ' + ${JSON.stringify(label)});
      option.scrollIntoView({block:'nearest'});
      const r = option.getBoundingClientRect();
      return {x:r.x+40,y:r.y+r.height/2};
    })()`);
    await page.command("Input.dispatchMouseEvent", {
      type: "mouseMoved",
      ...point,
    });
    await delay(150);
    await page.command("Input.dispatchMouseEvent", {
      type: "mousePressed",
      button: "left",
      clickCount: 1,
      ...point,
    });
    await page.command("Input.dispatchMouseEvent", {
      type: "mouseReleased",
      button: "left",
      clickCount: 1,
      ...point,
    });
    try {
      await page.wait(`!(${menuVisible})`, 2500);
    } catch (error) {
      console.log(
        "failed selection",
        agent,
        label,
        point,
        await page.evaluate(`(() => {
        const menu=document.querySelector('.web-session-model-select-menu');
        const hit=document.elementFromPoint(${point.x},${point.y});
        return {selected:document.querySelector('.model-select')?.innerText,menu:menu?.innerText,hit:hit?.outerHTML,errors:${JSON.stringify(errors)}};
      })()`),
      );
      const dir = path.join(env.root, "output/model-picker-regression");
      mkdirSync(dir, { recursive: true });
      await page.screenshot(path.join(dir, `${agent}-failure.png`));
      throw error;
    }
    await page.wait(
      "!document.querySelector('.model-option-detail')?.checkVisibility()",
      2500,
    );
    const selected = await page.evaluate(
      "document.querySelector('.model-select').innerText",
    );
    assert.ok(
      selected.trim(),
      "The selection must remain visible after dismissal",
    );
    // No navigation/reload between selections: the same selector must remain usable.
    await page.command("Input.dispatchMouseEvent", {
      type: "mouseMoved",
      x: 700,
      y: 300,
    });
    await delay(400);
    assert.equal(await page.evaluate(`Boolean(${menuVisible})`), false);
    const { items } = await env.request(
      `/projects/${env.project.id}/web-sessions`,
    );
    const expectedModel =
      agent === "pi"
        ? `${label === "fourth" ? "other-fixture" : "fixture"}/${label}`
        : agent === "claude"
          ? label.toLowerCase()
          : label;
    assert.equal(
      items.find((session) => session.id === activeSessionId)?.model,
      expectedModel,
    );
    const name = `${agent}: ${label}${query ? " / search" : ""}${composing ? " / unfinished composition" : ""}${recent ? " / recently used" : ""}`;
    passed.push(name);
    console.log("PASS", name);
  }

  for (const agent of ["pi", "codex", "devin", "claude"]) {
    const initialModel =
      agent === "pi"
        ? "fixture/first"
        : agent === "codex"
          ? codexModels[0]
          : agent === "claude"
            ? "sonnet"
            : "first";
    const { item: session } = await env.request(
      `/projects/${env.project.id}/web-sessions`,
      {
        worktreeId: env.worktree.id,
        agent,
        claudeRuntime: "",
        model: initialModel,
        reasoningEffort: "high",
        workflowMode: "default",
        permissionLevel: "standard",
        autoRetryEnabled: false,
        codexFastMode: false,
        title: `${agent} model selector regression`,
      },
    );
    activeSessionId = session.id;
    await page.navigate(
      `${env.origin}/#/project/${env.project.id}?tab=web&webSessionId=${session.id}`,
    );
    await page.wait(
      "document.querySelector('.model-select') && document.querySelector('[contenteditable=true]')",
      40000,
    );
    await delay(300);
    if (agent === "claude") {
      await selectOption(agent, "Opus");
      await selectOption(agent, "Haiku");
    } else {
      const labels = agent === "codex" ? codexModels : names;
      await selectOption(agent, labels[1]);
      await selectOption(agent, labels[2], { query: labels[2] });
      await selectOption(agent, labels[3], {
        query: labels[3],
        composing: true,
      });
      await selectOption(agent, labels[0], { recent: true });
      await openMenu();
      await page.click(searchInput);
      await page.evaluate(
        `document.querySelector(${JSON.stringify(searchInput)}).dispatchEvent(new CompositionEvent('compositionstart', {bubbles:true}))`,
      );
      await page.command("Input.dispatchMouseEvent", {
        type: "mouseMoved",
        x: 700,
        y: 300,
      });
      await delay(450);
      assert.equal(
        await page.evaluate(`Boolean(${menuVisible})`),
        true,
        "Typing must keep the catalog open",
      );
      await page.evaluate(`(() => {
        const input = document.querySelector(${JSON.stringify(searchInput)});
        input.dispatchEvent(new KeyboardEvent('keydown', {key:'Escape',bubbles:true,isComposing:true}));
      })()`);
      assert.equal(
        await page.evaluate(`Boolean(${menuVisible})`),
        true,
        "IME cancellation must not dismiss the catalog",
      );
      await page.evaluate(`(() => {
        const input = document.querySelector(${JSON.stringify(searchInput)});
        input.dispatchEvent(new CompositionEvent('compositionend', {bubbles:true}));
        input.dispatchEvent(new KeyboardEvent('keydown', {key:'Escape',bubbles:true}));
      })()`);
      await page.wait(`!(${menuVisible})`, 2500);
      passed.push(`${agent}: composition protection and Escape dismissal`);
      console.log("PASS", passed.at(-1));
    }
    await openMenu();
    await page.click("[contenteditable=true]");
    await page.command("Input.dispatchMouseEvent", {
      type: "mouseMoved",
      x: 700,
      y: 300,
    });
    await page.wait(`!(${menuVisible})`, 2500);
    passed.push(`${agent}: outside click after reopening`);
    console.log("PASS", passed.at(-1));
  }
  assert.deepEqual(
    errors,
    [],
    "The browser must not report uncaught exceptions",
  );
  console.log(JSON.stringify({ checks: passed.length, errors }, null, 2));
} finally {
  if (browser) await browser.close();
  if (env) await env.close();
}
