<template>
  <div class="wb-stack">
    <p class="wb-muted">
      仅用于已获授权的受控开发环境；固定镜像与代码版本，无网络访问。输出为清洗资产候选，须另行审核发布。
    </p>
    <p v-if="reason" class="wb-error" role="alert">{{ reason }}</p>
    <template v-if="profile">
      <p>
        执行配置：{{ profile.ID }} · v{{ profile.Version }}<br /><small>{{
          profile.Image
        }}</small>
      </p>
      <label class="wb-label"
        >代码资产<select v-model="codeID" aria-label="Sandbox代码资产">
          <option value="">选择包含 Python 代码的固定文本资产</option>
          <option v-for="a in assets" :key="key(a)" :value="key(a)">
            {{ a.name || a.asset_id }} · v{{ a.version }}
          </option>
        </select></label
      >
      <fieldset v-for="(input, index) in inputs" :key="index" class="wb-form">
        <label class="wb-label"
          >数据资产<select
            v-model="input.id"
            :aria-label="`Sandbox数据资产${index + 1}`"
          >
            <option value="">选择固定数据资产</option>
            <option v-for="a in assets" :key="key(a)" :value="key(a)">
              {{ a.name || a.asset_id }} · v{{ a.version }}
            </option>
          </select></label
        >
        <label class="wb-label"
          >输入文件名<input
            v-model="input.name"
            :aria-label="`Sandbox输入文件名${index + 1}`"
            pattern="[A-Za-z0-9][A-Za-z0-9._-]{0,127}"
        /></label>
        <button type="button" @click="inputs.splice(index, 1)">
          移除此输入
        </button>
      </fieldset>
      <button
        type="button"
        :disabled="inputs.length >= 15"
        @click="inputs.push({ id: '', name: `input${inputs.length + 1}.json` })"
      >
        添加 Sandbox 数据输入
      </button>
      <label class="wb-label"
        >最长运行秒数<input
          v-model.number="timeout"
          type="number"
          min="1"
          :max="Math.min(60, profile.MaxResources.timeout_seconds)"
          aria-label="Sandbox运行秒数"
      /></label>
      <p class="wb-muted">
        硬限制：{{ profile.MaxResources.cpu_millis }} 毫核 CPU、{{
          Math.floor(profile.MaxResources.memory_bytes / 1048576)
        }}
        MiB 内存、{{
          Math.floor(profile.MaxResources.disk_bytes / 1048576)
        }}
        MiB 临时空间、{{ profile.MaxResources.processes }} 进程、最多
        {{
          Math.min(16, profile.MaxResources.output_files)
        }}
        个输出文件。输入/输出各最多
        {{ Math.floor(profile.MaxResources.input_bytes / 1048576) }}/{{
          Math.floor(profile.MaxResources.output_bytes / 1048576)
        }}
        MiB。元数据会重新读取并固定真实摘要。
      </p>
    </template>
  </div>
</template>
<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { sandboxApi, type Profile } from "@/api/sandbox";
import { lifecycleApi, type AssetRef } from "@/api/lifecycle";
import type { NodeParameters } from "@/api/taskcontrol";
const props = defineProps<{ assets: AssetRef[] }>();
const profile = ref<Profile>(),
  reason = ref("正在读取已配置的执行环境…"),
  codeID = ref(""),
  inputs = ref<{ id: string; name: string }[]>([]),
  timeout = ref(30);
let generation = 0,
  alive = true;
const key = (a: AssetRef) => `${a.asset_id}:${a.version}`;
watch(
  () => JSON.stringify([codeID.value, inputs.value, timeout.value]),
  () => generation++,
);
onMounted(async () => {
  try {
    const r = (await sandboxApi.profiles()).data;
    if (!alive) return;
    if (!r?.enabled || !r.profile) {
      reason.value = r?.reason || "Sandbox未启用";
      return;
    }
    profile.value = r.profile;
    timeout.value = Math.min(30, 60, r.profile.MaxResources.timeout_seconds);
    reason.value = "";
  } catch {
    if (alive) reason.value = "无法读取Sandbox配置。";
  }
});
onBeforeUnmount(() => {
  alive = false;
  generation++;
});
async function prepare() {
  const epoch = ++generation;
  const snapshot = profile.value;
  if (!snapshot) throw new Error("Sandbox尚未启用。");
  if (
    !Number.isSafeInteger(timeout.value) ||
    timeout.value < 1 ||
    timeout.value > Math.min(60, snapshot.MaxResources.timeout_seconds)
  )
    throw new Error("运行时间超出固定配置。");
  const selected = [
    { id: codeID.value, name: "code" },
    ...inputs.value.map((v) => ({ ...v })),
  ];
  if (
    selected.some(
      (v) =>
        !v.id ||
        !/^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/.test(v.name) ||
        v.name.toLowerCase() === "runner.py",
    ) ||
    new Set(selected.map((v) => v.id)).size !== selected.length ||
    new Set(selected.map((v) => v.name)).size !== selected.length
  )
    throw new Error("请选择不重复的代码和数据资产，输入文件名须有效且不重复。");
  const current = (await sandboxApi.profiles()).data;
  if (!alive || epoch !== generation) return;
  if (
    !current?.enabled ||
    JSON.stringify(current.profile) !== JSON.stringify(snapshot)
  )
    throw new Error("执行配置已改变，请重新选择能力。");
  const bound: AssetRef[] = [],
    fixed: NonNullable<NodeParameters["sandbox"]>["inputs"] = [];
  for (const item of selected) {
    const candidate = props.assets.find((a) => key(a) === item.id);
    if (!candidate) throw new Error("所选来源不再可用。");
    const read = (
      await lifecycleApi.asset(candidate.asset_id, candidate.version)
    ).data?.ref;
    if (!alive || epoch !== generation) return;
    if (
      !read ||
      key(read) !== item.id ||
      read.kind !== candidate.kind ||
      read.status !== "available"
    )
      throw new Error("来源版本已变化。");
    const metadata = (
      await sandboxApi.metadata({
        asset_id: read.asset_id,
        version: read.version,
      })
    ).data;
    if (!alive || epoch !== generation) return;
    if (
      !metadata ||
      metadata.Ref.asset_id !== read.asset_id ||
      metadata.Ref.version !== read.version ||
      !/^sha256:[a-f0-9]{64}$/.test(metadata.SHA256) ||
      !Number.isSafeInteger(metadata.SizeBytes) ||
      metadata.SizeBytes < 0
    )
      throw new Error("无法固定来源摘要。");
    bound.push(read);
    fixed.push({
      name: item.name,
      ref: metadata.Ref,
      sha256: metadata.SHA256,
      size_bytes: metadata.SizeBytes,
    });
  }
  const resources = {
    ...snapshot.MaxResources,
    timeout_seconds: timeout.value,
    output_files: Math.min(16, snapshot.MaxResources.output_files),
  };
  if (
    fixed[0]!.size_bytes === 0 ||
    fixed.reduce((sum, v) => sum + v.size_bytes, 0) > resources.input_bytes
  )
    throw new Error("代码为空或总输入超过资源预算。");
  return {
    assets: bound,
    parameters: {
      sandbox: {
        profile_id: snapshot.ID,
        profile_version: snapshot.Version,
        image_sha256: snapshot.Image.slice(snapshot.Image.lastIndexOf("@") + 1),
        code: fixed[0]!,
        inputs: fixed.slice(1),
        resources,
      },
    } satisfies NodeParameters,
  };
}
defineExpose({ prepare });
</script>
