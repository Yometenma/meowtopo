/* Application bindings and startup. */
let searchMatches = [];
let searchIndex = 0;

function applyThemeMode() {
  const manual = localStorage.theme;
  const dark = manual
    ? manual === "dark"
    : matchMedia("(prefers-color-scheme: dark)").matches;
  document.documentElement.classList.toggle("dark", dark);
  return dark;
}

function focusSearchResult(announce = false) {
  const id = searchMatches[searchIndex];
  const device = devices.find((x) => x.id === id);
  if (!device) return;
  cy.nodes().removeClass("found");
  const node = cy.$id(String(id));
  node.addClass("found");
  cy.animate({ center: { eles: node }, zoom: 1.4 }, { duration: 250 });
  node.select();
  if (announce && searchMatches.length > 1) {
    toast(`搜索结果 ${searchIndex + 1}/${searchMatches.length} · ${nameOf(device)}`);
  }
}

function bind() {
  $("#scanBtn").onclick = startScan;
  $("#activityRefresh").onclick = loadActivity;
  [
    "managerSearch",
    "managerStatus",
    "managerType",
    "managerVisibility",
  ].forEach((id) => ($("#" + id).oninput = renderManager));
  $("#managerSelectAll").onchange = (e) => {
    managerFiltered.forEach((d) =>
      e.target.checked
        ? selectedDevices.add(d.id)
        : selectedDevices.delete(d.id),
    );
    renderManager();
  };
  document
    .querySelectorAll("[data-batch]")
    .forEach(
      (button) => (button.onclick = () => runBatch(button.dataset.batch)),
    );
  $("#batchParentSave").onclick = () => runBatch("set_parent");
  $("#layoutBtn").onclick = () =>
    cy
      ?.layout({ name: "cose", animate: true, duration: 350, padding: 55 })
      .run();
  $("#fitBtn").onclick = () => cy?.fit(undefined, 50);
  $("#closeDrawer").onclick = () => $("#drawer").classList.remove("open");
  $("#emptySetup").onclick = wizard;
  $("#wizardSave").onclick = saveWizard;
  $("#settingsSave").onclick = saveSettings;
  $("#search").oninput = (e) => {
    if (!cy) return;
    const q = e.target.value.trim().toLowerCase();
    searchMatches = q
      ? devices
          .filter(
            (x) =>
              nameOf(x).toLowerCase().includes(q) ||
              (x.current_ip || "").includes(q),
          )
          .map((x) => x.id)
      : [];
    searchIndex = 0;
    cy.nodes().removeClass("found");
    if (!searchMatches.length) return;
    focusSearchResult();
  };
  $("#search").onkeydown = (e) => {
    if (e.key !== "Enter" || !searchMatches.length) return;
    e.preventDefault();
    searchIndex = (searchIndex + 1) % searchMatches.length;
    focusSearchResult(true);
  };
  $("#addBtn").onclick = () => $("#manualDialog").showModal();
  $("#manualSave").onclick = async (e) => {
    e.preventDefault();
    try {
      await api("/api/devices", {
        method: "POST",
        body: JSON.stringify({
          Name: $("#manualName").value,
          Type: $("#manualType").value,
          Notes: $("#manualNotes").value,
        }),
      });
      $("#manualDialog").close();
      await refresh(true);
      toast("手工节点已添加");
    } catch (x) {
      toast(x.message);
    }
  };
  $("#backupTab").onclick = () => showPane("backup");
  $("#aboutTab").onclick = async () => {
    showPane("about");
    const v = await api("/api/version");
    $("#versionText").textContent = `版本 ${v.version}`;
  };
  $("#restoreFile").onchange = async (e) => {
    if (!e.target.files[0]) return;
    if (!confirm("恢复会替换当前数据库，是否继续？")) return;
    try {
      await fetch("/api/restore", {
        method: "POST",
        headers: { "Content-Type": "application/zip" },
        body: e.target.files[0],
      }).then(async (r) => {
        if (!r.ok) throw new Error((await r.json()).error.message);
      });
      toast("恢复完成，请刷新页面");
      setTimeout(() => location.reload(), 1000);
    } catch (x) {
      toast(x.message);
    }
  };
  appExtensions.bind();
  appExtensions.snmpBind();
}
function showPane(v) {
  $("#settingsMain").classList.toggle("hidden", v !== "main");
  $("#notificationPane").classList.toggle("hidden", v !== "notification");
  $("#backupPane").classList.toggle("hidden", v !== "backup");
  $("#aboutPane").classList.toggle("hidden", v !== "about");
  document
    .querySelectorAll(".tabs button")
    .forEach((b) => b.classList.remove("active"));
  const active =
    {
      notification: $("#notificationTab"),
      backup: $("#backupTab"),
      about: $("#aboutTab"),
    }[v] || document.querySelector(".tabs button");
  active?.classList.add("active");
}
document.addEventListener("DOMContentLoaded", async () => {
  applyThemeMode();
  matchMedia("(prefers-color-scheme: dark)").addEventListener(
    "change",
    () => {
      if (localStorage.theme) return;
      applyThemeMode();
      applyTopologyTheme();
      applyIconStyle();
    },
  );
  bindAccount();
  try {
    await requireAccount();
  } catch (e) {
    toast(e.message);
    return;
  }
  ensureNotificationUI();
  document.querySelector(".tabs button").onclick = () => showPane("main");
  bind();
  $("#layoutBtn").onclick = () => runTopologyLayout(true);
  $("#themeBtn").onclick = () => {
    localStorage.theme = document.documentElement.classList.contains("dark")
      ? "light"
      : "dark";
    applyThemeMode();
    applyTopologyTheme();
    applyIconStyle();
    refresh();
  };
  initCy();
  applyTopologyTheme();
  applyIconStyle();
  applyAccountPermissions();
  try {
    settings = can(permission.settings)
      ? await api("/api/settings")
      : { initialized: "true" };
    await refresh();
    showProgress(await api("/api/scan/status"));
    connectEvents();
    if (can(permission.settings) && settings.initialized !== "true") wizard();
  } catch (e) {
    toast(e.message);
  }
});
