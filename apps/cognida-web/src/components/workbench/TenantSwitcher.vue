<template>
  <div class="tenant-switcher">
    <label class="wb-label" for="workspace">{{ wt("工作空间") }}</label>
    <select
      id="workspace"
      :value="auth.currentTenant?.id"
      :disabled="busy"
      @change="switchTo(Number(($event.target as HTMLSelectElement).value))"
    >
      <option v-if="!auth.tenants.length" :value="auth.currentTenant?.id">
        当前工作空间
      </option>
      <option
        v-for="tenant in auth.tenants"
        :key="tenant.id"
        :value="tenant.id"
      >
        {{ tenant.name }} · {{ tenant.role }}
      </option>
    </select>
    <button type="button" @click="creating = !creating">
      {{ creating ? "取消创建" : "＋ 新建工作空间" }}
    </button>
    <form v-if="creating" @submit.prevent="create">
      <label class="wb-label"
        >{{ wt("名称")
        }}<input v-model="name" required minlength="2" maxlength="255" /></label
      ><label class="wb-label"
        >{{ wt("业务领域")
        }}<input
          v-model="business"
          required
          maxlength="255"
          placeholder="例如：客户服务" /></label
      ><button :disabled="busy">{{ wt("创建并切换") }}</button>
    </form>
    <p v-if="error" role="alert" class="tenant-error">{{ error }}</p>
  </div>
</template>
<script setup lang="ts">
import { wt } from "@/i18n/workbench";

import { ref, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "@/stores/auth";
import { http } from "@/utils/request";
import type { Tenant } from "@/types";
const auth = useAuthStore(),
  router = useRouter(),
  busy = ref(false),
  creating = ref(false),
  name = ref(""),
  business = ref(""),
  error = ref("");
onMounted(async () => {
  try {
    await auth.fetchTenants();
  } catch (e) {
    error.value = e instanceof Error ? e.message : "工作空间加载失败";
  }
});
async function switchTo(id: number) {
  const tenant = auth.tenants.find((t) => t.id === id);
  if (!tenant) return;
  busy.value = true;
  error.value = "";
  try {
    await auth.setCurrentTenant(tenant);
    await router.replace("/home");
  } catch (e) {
    error.value = e instanceof Error ? e.message : "切换失败";
  } finally {
    busy.value = false;
  }
}
async function create() {
  busy.value = true;
  error.value = "";
  try {
    const result = await http.post<Tenant>(
      "/user/tenants",
      { name: name.value, business: business.value },
      { headers: { "Idempotency-Key": crypto.randomUUID() } },
    );
    await auth.fetchTenants();
    if (result.data) {
      await switchTo(result.data.id);
      creating.value = false;
      name.value = "";
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : "创建失败";
  } finally {
    busy.value = false;
  }
}
</script>
<style scoped>
.tenant-switcher {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 2px 16px;
  font-size: 12px;
}
.tenant-switcher form {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.tenant-switcher button {
  text-align: left;
  background: transparent;
  padding: 5px;
}
.tenant-error {
  color: var(--danger);
  font-size: 11px;
}
</style>
