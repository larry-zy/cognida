import { describe, it, expect } from "vitest";
import { mount } from "@vue/test-utils";
import DecisionCard from "./DecisionCard.vue";
describe("decision provenance", () => {
  it("does not imply missing verification or confidence passed", () => {
    const wrapper = mount(DecisionCard, {
      props: { refs: [], summary: "Revenue changed" },
    });
    expect(wrapper.text()).toContain("未提供验证结论");
    expect(wrapper.text()).toContain("未评估");
    expect(wrapper.text()).toContain("不能认定为已验证结论");
  });
});
