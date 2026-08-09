# 第三方资产声明

本目录内嵌的 `tokenizer.json` 为 **DeepSeek-V3** 官方分词器词表（BPE，vocab 128000 /
merges 127741），仅用于在服务端离线估算 token 数（不含任何模型权重）。

DeepSeek-V3 的代码与配置以 **MIT License** 发布，允许在保留版权与许可声明的前提下再分发。

```
MIT License

Copyright (c) 2023 DeepSeek

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

加载时对该 JSON 做了一处**计数中性**的补丁：剔除 pre_tokenizer 正则里 Go RE2 不支持的
负向先行断言分支 `\s+(?!\S)`（它匹配行尾/串尾空白，与紧随的兜底分支 `\s+` 范围重叠，删除
不改变 token 数）。词表与合并规则逐字节保留。详见 `bpecount.go` 包注释。
