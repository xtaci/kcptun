# Quantum Permutation Pad (量子置换密码本)

[![GoDoc][1]][2] [![Go Report Card][3]][4] [![CreatedAt][5]][6]

[English](README.md)

[1]: https://godoc.org/github.com/xtaci/qpp?status.svg
[2]: https://pkg.go.dev/github.com/xtaci/qpp
[3]: https://goreportcard.com/badge/github.com/xtaci/qpp
[4]: https://goreportcard.com/report/github.com/xtaci/qpp
[5]: https://img.shields.io/github/created-at/xtaci/qpp
[6]: https://img.shields.io/github/created-at/xtaci/qpp

[Quantum Permutation Pad](https://link.springer.com/content/pdf/10.1140/epjqt/s40507-023-00164-3.pdf)（QPP）是一种以量子力学为根基、面向安全通信的加密协议。它把叠加与纠缠等量子特性引入置换操作，构建比经典方案更庞大也更难穷举的密钥空间。本篇 README 旨在用更贴近日常研发语境的方式，概览 QPP 的核心概念、优势、实现细节以及最佳实践。

文中所称的 pad 即“置换密码本”（Permutation Pad），下文为便于阅读将以“密码本”指代，并在首次出现时标注 pad。

## Quantum Permutation Pad 的核心概念

1. **量子力学支撑**：QPP 的安全性基于叠加（量子比特可同时处于多种态）与纠缠（相隔万里的量子比特仍保持关联）等基本原理，确保密钥演化具备不可克隆、不可预测的特性。
2. **量子比特 (Qubits)**：不同于只能表示 0 或 1 的经典比特，量子比特可处于 $|0\rangle$、 $|1\rangle$ 及其任意叠加态，因而承载的密钥信息维度更高。
3. **置换操作**：在 QPP 中，“置换”指对明文的解读方式进行重映射，而非经典意义上反复打乱密钥。一个 8 位字节拥有 $P_{256}$ 种置换可能，即
```256! =857817775342842654119082271681232625157781520279485619859655650377269452553147589377440291360451408450375885342336584306157196834693696475322289288497426025679637332563368786442675207626794560187968867971521143307702077526646451464709187326100832876325702818980773671781454170250523018608495319068138257481070252817559459476987034665712738139286205234756808218860701203611083152093501947437109101726968262861606263662435022840944191408424615936000000000000000000000000000000000000000000000000000000000000000000```
4. **密钥空间扩展**：经典 8 位系统仅有 256 个可能密钥，而 QPP 的量子置换空间暴涨至 256! 个量子算子，暴力破解成本呈阶乘级增长。
5. **实现路径**：该协议既能以矩阵形式完成经典实现，也能映射为量子电路中的置换门。

## 应用与优势

- **高安全冗余**：阶乘级密钥空间与量子特性叠加，显著提升抵抗穷举及已知明文攻击的能力。
- **量子时代兼容性**：面对量子计算对 RSA、ECC 等传统算法的威胁，QPP 提供天然抗量子的候选方案。
- **敏感数据护航**：适用于量子网络、分布式系统以及对安全性要求严苛的场景。

## 使用示例
内部 PRNG（不推荐）
```golang
func main() {
    seed := make([]byte, 32)
    io.ReadFull(rand.Reader, seed)

    qpp := NewQPP(seed, 977) // 这里的密码本（pad）数量是一个质数

    msg := make([]byte, 65536)
    io.ReadFull(rand.Reader, msg)

    qpp.Encrypt(msg)
    qpp.Decrypt(msg)
}
```

外部 PRNG 与共享密码本（pads）（**推荐**）
```golang
func main() {
    seed := make([]byte, 32)
    io.ReadFull(rand.Reader, seed)

    qpp := NewQPP(seed, 977)

    msg := make([]byte, 65536)
    io.ReadFull(rand.Reader, msg)

    rand_enc := qpp.CreatePRNG(seed)
    rand_dec := qpp.CreatePRNG(seed)

    qpp.EncryptWithPRNG(msg, rand_enc)
    qpp.DecryptWithPRNG(msg, rand_dec)
}
```

`NewQPP` 生成的置换密码本如下所示（采用[轮换表示法](https://zh.wikipedia.org/wiki/%E7%BD%AE%E6%8D%A2#%E8%BD%AE%E6%8D%A2%E8%A1%A8%E7%A4%BA%E6%B3%95)）：
```
(0 4 60 108 242 196)(1 168 138 16 197 29 57 21 22 169 37 74 205 33 56 5 10 124 12 40 8 70 18 6 185 137 224)(2 64 216 178 88)(3 14 98 142 128 30 102 44 158 34 72 38 50 68 28 154 46 156 254 41 218 204 161 194 65)(7 157 101 181 141 121 77 228 105 206 193 155 240 47 54 78 110 90 174 52 207 233 248 167 245 199 79 144 162 149 97
140 111 126 170 139 175 119 189 171 215 55 89 81 23 134 106 251 83 15 173 250 147 217 115 229 99 107 223 39 244 246
 225 252 226 203 235 236 253 43 188 209 145 184 91 31 49 84 210 117 59 133 129 75 150 127 200 130 132 247 159 241
255 71 120 63 249 201 212 131 95 222 238 125 237 109 186 213 151 176 143 202 179 232 103 148 191 239)(9 20 113 73 69 160 114 122 164 17 208 58 116 36 26 96 24)(11 80 32 152 146 82 53 62 66 76 86 51 112 221 27 163 180 214 123 219 234)
(13 42 166 25 165)(19 172 177 230 198 45 61 104 136 100 182 85 153 35 192 48 220 94 190 118 195)(67)(87 92 93 227 211)(135)(183 243)(187)(231)
```
![circular](https://github.com/user-attachments/assets/3fa50405-1b4e-4679-a495-548850c4315b)

## 本实现的安全性设计
整体安全性可视为 **1683 位** 对称加密强度。

在 8 位量子比特系统中，置换矩阵的数量取决于所提供的随机种子，并经随机程序选定。

<img width="867" alt="image" src="https://github.com/user-attachments/assets/93ce8634-5300-47b1-ba1b-e46d9b46b432">

置换密码本（pad）可以用[轮换表示法](https://zh.wikipedia.org/wiki/%E7%BD%AE%E6%8D%A2#%E8%BD%AE%E6%8D%A2%E8%A1%A8%E7%A4%BA%E6%B3%95)写为： $\sigma =(1\ 2\ 255)(3\ 36)(4\ 82\ 125)(...)$，其中置换元素无法像传统[流密码](https://zh.wikipedia.org/wiki/%E6%B5%81%E5%AF%86%E7%A0%81)那样通过两次 **异或 (XOR)** 还原，大幅提升分析难度。

#### 局部性与随机性
完全随机的置换会破坏[局部性原理](https://zh.wikipedia.org/wiki/%E5%B1%80%E9%83%A8%E6%80%A7%E5%8E%9F%E7%90%86)，影响缓存与流水线表现。为兼顾性能与安全，我们选择“每 8 字节切换一次密码本（pad）”的策略，在确保熵值的前提下维持一定局部性。

![349804164-3f6da444-a9f4-4d0a-b190-d59f2dca9f00](https://github.com/user-attachments/assets/2358766e-a0d3-4c21-93cb-c221aa0cece0)

上图展示了每个字节切换密码本的性能瓶颈，以及“8 字节一换”策略在吞吐与随机度之间取得的平衡。

可直接下载 https://github.com/xtaci/kcptun/releases 并启用 ```-QPP``` 选项体验。

#### 性能
在现代 CPU 上，最新优化已可稳定突破 1GB/s。

![348621244-4061d4a9-e7fa-43f5-89ef-f6ef6c00a2e7](https://github.com/user-attachments/assets/78952157-df39-4088-b423-01f45548b782)

## 设置密码本（pads）的安全注意事项

密码本数量（pads）最好与 8 互素（coprime）。实验显示 PRNG 内部与 8 相关的隐藏结构会在非互素时削弱随机性。

![88d8de919445147f5d44ee059cca371](https://github.com/user-attachments/assets/9e1a160d-5433-4e24-9782-2ae88d87453d)

我们以真实数据（加密后的圣经文本）对比 64 与 15 份密码本（pads）的效果：

**密码本数（Pads）= 64**： $GCD(64,8) = 8, \chi^2 =3818$

![348794146-4f6d5904-2663-46d7-870d-9fd7435df4d0](https://github.com/user-attachments/assets/e2a67bad-7d10-46e4-8d23-9866918ef04b)

**密码本数（Pads）= 15**： $GCD(15,8) = 1, \chi^2 =230$，互素配置带来更理想的均匀度。

![348794204-accd3992-a56e-4059-a472-39ba5ad75660](https://github.com/user-attachments/assets/a6fd2cb8-7517-4627-8fd6-0cf29711b09d)

> 更多密码本数量（pad count）与卡方分析结果可见：https://github.com/xtaci/qpp/blob/main/misc/chi-square.csv


**[卡方分布](https://zh.wikipedia.org/wiki/%E5%8D%A1%E6%96%B9%E5%88%86%E5%B8%83)** 结果进一步证明，选择与 8 互素的密码本数量可显著提升随机性。

## 结论

Quantum Permutation Pad 借助量子置换实现面向未来的安全通信。随着量子计算与量子网络持续进化，QPP 这类协议将在新一代安全体系中扮演关键角色。

## 贡献

欢迎贡献！欢迎通过 issue 或 pull request 反馈想法、修复缺陷或提出新特性。

## 许可证

本项目采用 GPLv3 许可证，详情参阅 [LICENSE](LICENSE)。

## 参考资料

更多细节可查阅[研究论文](https://link.springer.com/content/pdf/10.1140/epjqt/s40507-023-00164-3.pdf)。

## 致谢

特别感谢研究论文的作者在 Quantum Permutation Pad 方面的开创性工作。
