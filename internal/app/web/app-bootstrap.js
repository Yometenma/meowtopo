/* Application bindings and startup. */
function bind() {
  $("#scanBtn").onclick = startScan;
  $("#manageBtn").onclick = openManager;
  $("#activityBtn").onclick = openActivity;
  $("#activityClose").onclick = () => $("#activityDialog").close();
  $("#activityRefresh").onclick = loadActivity;
  $("#managerClose").onclick = () => $("#managerDialog").close();
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
  $("#settingsBtn").onclick = openSettings;
  $("#emptySetup").onclick = wizard;
  $("#wizardSave").onclick = saveWizard;
  $("#settingsSave").onclick = saveSettings;
  $("#themeBtn").onclick = () => {
    document.documentElement.classList.toggle("dark");
    localStorage.theme = document.documentElement.classList.contains("dark")
      ? "dark"
      : "light";
  };
  $("#search").oninput = (e) => {
    if (!cy) return;
    const q = e.target.value.toLowerCase();
    cy.nodes().removeClass("found");
    const d = devices.find(
      (x) => nameOf(x).toLowerCase().includes(q) || x.current_ip.includes(q),
    );
    if (d && q) {
      const n = cy.$id(String(d.id));
      cy.animate({ center: { eles: n }, zoom: 1.4 }, { duration: 250 });
      n.select();
    }
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
  if (
    localStorage.theme === "dark" ||
    (!localStorage.theme && matchMedia("(prefers-color-scheme:dark)").matches)
  )
    document.documentElement.classList.add("dark");
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
    document.documentElement.classList.toggle("dark");
    localStorage.theme = document.documentElement.classList.contains("dark")
      ? "dark"
      : "light";
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
