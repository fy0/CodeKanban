<template>
  <div class="user-guide-page">
    <n-page-header>
      <template #title>
        <n-space align="center">
          <n-icon size="24">
            <BookOutline />
          </n-icon>
          <span>使用指引</span>
        </n-space>
      </template>
      <template #extra>
        <n-button quaternary size="small" @click="goBack">
          返回项目列表
        </n-button>
      </template>
    </n-page-header>

    <div class="guide-content">
      <!-- 欢迎介绍 -->
      <n-card title="欢迎使用 AICode Kanban">
        <n-space vertical size="large">
          <n-text>
            AICode Kanban 是一个面向 <n-text strong>AI 编程时代</n-text>的效率工具，
            专为解决 <n-text strong type="success">CLI AI 编程工具的多项目、多终端管理痛点</n-text>而设计。
          </n-text>

          <n-alert type="warning" title="AI 编程时代的挑战">
            使用 Claude Code、Codex、Droid、Gemini Code 等 CLI AI 编程工具时，经常需要在多个项目、多个分支间频繁切换。
            传统的终端窗口杂乱无章，项目路径难以追踪，多个 AI CLI 工具同时运行更是混乱不堪。
            AICode Kanban 让你在一个界面中掌控一切！
          </n-alert>

          <n-alert type="success" title="核心优势">
            <n-space vertical size="small">
              <div>
                <n-text strong>🚀 多项目快速切换</n-text>
                <n-text depth="3" style="display: block; margin-top: 4px">
                  在一个界面中管理所有项目，随时切换到任意项目的任意分支工作区，无需关闭编辑器或终端。
                  告别繁琐的目录切换和项目查找。
                </n-text>
              </div>
              <n-divider style="margin: 8px 0" />
              <div>
                <n-text strong>💻 终端集中管理</n-text>
                <n-text depth="3" style="display: block; margin-top: 4px">
                  所有项目的终端会话集中在一个界面，支持多标签页管理。
                  为不同项目、不同工作区开启独立终端，一键切换，告别杂乱的终端窗口。
                </n-text>
              </div>
            </n-space>
          </n-alert>

          <n-alert type="info" title="Git Worktree 技术加持">
            通过 Git Worktree 技术，让你在同一个项目的不同分支间无缝切换，
            无需频繁 checkout 和 stash，每个工作区独立维护自己的文件状态。
          </n-alert>
        </n-space>
      </n-card>

      <!-- 快速开始 -->
      <n-card title="快速开始">
        <n-steps vertical :current="null">
          <n-step title="创建项目">
            <n-space vertical>
              <n-text depth="3">
                点击项目列表页右上角的「添加项目」按钮，填写项目名称和项目路径。
              </n-text>
              <n-text depth="3" type="info">
                <n-icon size="16"><AlertCircleOutline /></n-icon>
                建议使用 Git 仓库根目录，否则无法使用工作区（Worktree）功能。
              </n-text>
            </n-space>
          </n-step>

          <n-step title="创建工作区（可选）">
            <n-space vertical>
              <n-text depth="3">
                如果项目是 Git 仓库，可以点击「Worktrees」标签页，然后点击「新建」按钮，选择基础分支创建工作区。
              </n-text>
              <n-text depth="3">
                系统会自动在项目目录下创建独立的工作区文件夹（使用 git worktree）。
              </n-text>
              <n-text depth="3" type="info">
                <n-icon size="16"><AlertCircleOutline /></n-icon>
                这一步不是必需的。如果你认为对于当前任务，AI 工具对主分支的共同编辑并不会引起混乱，无需创建工作区。
              </n-text>
            </n-space>
          </n-step>

          <n-step title="管理任务">
            <n-space vertical>
              <n-text depth="3">
                在看板中创建任务，拖拽任务卡片调整状态（待办 → 进行中 → 已完成）。
              </n-text>
              <n-text depth="3">
                任务可以关联到特定的工作区和分支。
              </n-text>
            </n-space>
          </n-step>

          <n-step title="使用终端">
            <n-space vertical>
              <n-text depth="3">
                在工作区卡片中点击「打开终端」按钮，会自动打开终端并切换到该工作区目录。
              </n-text>
              <n-text depth="3">
                支持多标签页管理，可以为不同工作区打开独立的终端。
              </n-text>
              <n-text depth="3" type="success">
                <n-icon size="16"><AlertCircleOutline /></n-icon>
                特别适合运行 CLI AI 编程工具（如 claude code、codex、droid、gemini code 等），
                在一个界面中集中管理所有 AI 编程会话，随时切换项目和工作区。
              </n-text>
            </n-space>
          </n-step>
        </n-steps>
      </n-card>

      <!-- 核心功能 -->
      <n-card title="核心功能详解">
        <n-collapse>
          <n-collapse-item title="项目管理" name="projects">
            <n-space vertical>
              <n-h3>创建项目</n-h3>
              <n-text>在项目列表页点击「添加项目」，输入项目信息：</n-text>
              <n-ul>
                <n-li><strong>项目名称：</strong>便于识别的显示名称</n-li>
                <n-li><strong>项目路径：</strong>项目目录的绝对路径（建议使用 Git 仓库根目录）</n-li>
                <n-li><strong>默认分支：</strong>通常是 main 或 master（仅当项目是 Git 仓库时需要）</n-li>
                <n-li><strong>描述：</strong>可选的项目说明</n-li>
              </n-ul>
              <n-alert type="info" title="关于项目路径">
                项目路径不强制要求是 Git 仓库，但如果不是 Git 仓库，将无法使用工作区（Worktree）管理、分支管理等 Git 相关功能。
              </n-alert>

              <n-h3>编辑项目</n-h3>
              <n-text>在项目卡片右上角点击更多按钮（···），选择「编辑」可以修改项目元数据。</n-text>

              <n-h3>删除项目</n-h3>
              <n-text depth="3" type="warning">
                <n-icon size="16"><AlertCircleOutline /></n-icon>
                删除项目只会移除数据库记录，不会删除磁盘上的 Git 仓库。
              </n-text>
            </n-space>
          </n-collapse-item>

          <n-collapse-item title="工作区（Worktree）管理" name="worktrees">
            <n-space vertical>
              <n-alert type="warning" title="前置要求">
                工作区（Worktree）功能需要项目是一个有效的 Git 仓库。如果项目路径不是 Git 仓库，该功能将不可用。
              </n-alert>

              <n-h3>什么是 Worktree？</n-h3>
              <n-text>
                Git Worktree 允许你在同一个仓库中同时签出多个分支到不同的目录。
                这样你可以在不同分支间快速切换，而无需 stash 或 commit 未完成的工作。
              </n-text>

              <n-h3>创建工作区</n-h3>
              <n-ol>
                <n-li>点击「新建工作区」</n-li>
                <n-li>选择已有分支，或创建新分支</n-li>
                <n-li>系统会在项目目录下创建 <n-text code>.worktrees/分支名</n-text> 目录</n-li>
              </n-ol>

              <n-h3>工作区操作</n-h3>
              <n-ul>
                <n-li><strong>在编辑器中打开：</strong>使用配置的编辑器（VS Code、Cursor 等）打开工作区目录</n-li>
                <n-li><strong>在终端打开：</strong>打开新的终端标签页，工作目录自动切换到该工作区</n-li>
                <n-li><strong>删除工作区：</strong>会同时删除工作区目录和 Git worktree 记录</n-li>
              </n-ul>

              <n-alert type="warning" title="注意事项">
                <n-ul>
                  <n-li>同一个分支不能被多个工作区同时签出</n-li>
                  <n-li>删除工作区前请确保重要修改已提交</n-li>
                </n-ul>
              </n-alert>
            </n-space>
          </n-collapse-item>

          <n-collapse-item title="任务看板系统" name="kanban">
            <n-space vertical>
              <n-h3>看板布局</n-h3>
              <n-text>看板分为三列：</n-text>
              <n-ul>
                <n-li><strong>待办（Todo）：</strong>计划要做的任务</n-li>
                <n-li><strong>进行中（In Progress）：</strong>正在开发的任务</n-li>
                <n-li><strong>已完成（Done）：</strong>已完成的任务</n-li>
              </n-ul>

              <n-h3>创建任务</n-h3>
              <n-text>点击列顶部的「+」按钮，填写任务信息：</n-text>
              <n-ul>
                <n-li><strong>标题：</strong>任务的简短描述</n-li>
                <n-li><strong>描述：</strong>详细的任务说明（支持 Markdown）</n-li>
                <n-li><strong>优先级：</strong>低/中/高，用颜色标识</n-li>
                <n-li><strong>关联分支：</strong>可选，标记任务所属分支</n-li>
              </n-ul>

              <n-h3>拖拽排序</n-h3>
              <n-text>可以：</n-text>
              <n-ul>
                <n-li>在同一列内上下拖动任务卡片，调整优先级顺序</n-li>
                <n-li>跨列拖动任务卡片，改变任务状态</n-li>
              </n-ul>

              <n-h3>任务操作</n-h3>
              <n-ul>
                <n-li>点击任务卡片查看详情和完整描述</n-li>
                <n-li>点击编辑按钮修改任务信息</n-li>
                <n-li>点击删除按钮移除任务</n-li>
              </n-ul>
            </n-space>
          </n-collapse-item>

          <n-collapse-item title="集成终端" name="terminal">
            <n-space vertical>
              <n-h3>打开终端</n-h3>
              <n-text>在工作区卡片中点击「打开终端」按钮，会自动打开终端面板并切换到该工作区的目录。</n-text>

              <n-h3>折叠/展开终端</n-h3>
              <n-text>按 <n-text code>`</n-text> 键（反引号）可以快速切换终端面板的展开/折叠状态，方便查看代码或执行命令。</n-text>

              <n-h3>多标签管理</n-h3>
              <n-ul>
                <n-li>点击终端面板右侧的「+」创建新的终端标签页</n-li>
                <n-li>每个标签页都是独立的 Shell 会话</n-li>
                <n-li>标签页会显示当前工作目录名称</n-li>
                <n-li>点击标签右侧的「×」关闭终端</n-li>
              </n-ul>

              <n-h3>终端功能</n-h3>
              <n-ul>
                <n-li>支持完整的 Shell 命令（CMD、PowerShell、Bash 等）</n-li>
                <n-li>支持颜色输出和 ANSI 转义序列</n-li>
                <n-li>支持 Ctrl+C 中断命令</n-li>
                <n-li>支持复制粘贴（右键菜单）</n-li>
                <n-li>支持搜索（Ctrl+F）</n-li>
              </n-ul>

              <n-h3>自动切换目录</n-h3>
              <n-text>从工作区卡片点击「打开终端」时，新终端会自动切换到该工作区的目录。</n-text>
            </n-space>
          </n-collapse-item>

          <n-collapse-item title="分支管理" name="branches">
            <n-space vertical>
              <n-h3>查看分支</n-h3>
              <n-text>在项目页面顶部导航栏点击「分支管理」查看所有本地和远程分支。</n-text>

              <n-h3>创建分支</n-h3>
              <n-text>可以从已有分支创建新分支，用于开发新功能。</n-text>

              <n-h3>切换分支</n-h3>
              <n-text depth="3" type="warning">
                <n-icon size="16"><AlertCircleOutline /></n-icon>
                推荐通过创建工作区的方式使用不同分支，而不是直接 checkout。
              </n-text>
            </n-space>
          </n-collapse-item>

          <n-collapse-item title="笔记本" name="notepad">
            <n-space vertical>
              <n-h3>快速打开</n-h3>
              <n-text>按 <n-text code>1</n-text> 键可以快速打开/关闭笔记本面板</n-text>

              <n-h3>项目笔记</n-h3>
              <n-text>每个项目都有独立的笔记本，用于记录：</n-text>
              <n-ul>
                <n-li>项目相关的开发笔记</n-li>
                <n-li>重要的命令或脚本</n-li>
                <n-li>问题排查记录</n-li>
                <n-li>待办事项列表</n-li>
              </n-ul>

              <n-h3>功能特点</n-h3>
              <n-ul>
                <n-li>支持 Markdown 格式</n-li>
                <n-li>实时预览</n-li>
                <n-li>自动保存</n-li>
              </n-ul>
            </n-space>
          </n-collapse-item>
        </n-collapse>
      </n-card>

      <!-- 使用技巧 -->
      <n-card title="使用技巧与最佳实践">
        <n-space vertical size="large">
          <div>
            <n-h3>
              <n-icon size="20"><BulbOutline /></n-icon>
              高效的多分支开发工作流
            </n-h3>
            <n-ol>
              <n-li>
                <strong>主开发分支：</strong>为主要开发分支（如 develop）创建一个常驻工作区
              </n-li>
              <n-li>
                <strong>功能分支：</strong>每个新功能创建独立的分支和工作区
              </n-li>
              <n-li>
                <strong>快速切换：</strong>需要在不同功能间切换时，直接在编辑器中打开对应工作区即可
              </n-li>
              <n-li>
                <strong>紧急修复：</strong>遇到紧急 bug，可以立即为 hotfix 分支创建工作区，无需中断当前工作
              </n-li>
            </n-ol>
          </div>

          <n-divider />

          <div>
            <n-h3>
              <n-icon size="20"><RocketOutline /></n-icon>
              提升效率的小技巧
            </n-h3>
            <n-space vertical>
              <div>
                <n-text strong>1. 使用快捷键提升效率</n-text>
                <n-text depth="3" style="display: block; margin-top: 8px">
                  按 <n-text code>`</n-text> 键（反引号）快速展开/折叠终端面板，
                  按 <n-text code>1</n-text> 键快速打开/关闭笔记本面板。
                  这样可以在不离开键盘的情况下快速切换工作区域。
                </n-text>
              </div>

              <div>
                <n-text strong>2. 自定义编辑器</n-text>
                <n-text depth="3" style="display: block; margin-top: 8px">
                  在设置中配置你习惯的编辑器（VS Code、Cursor、WebStorm 等），
                  点击「在编辑器中打开」时会自动使用该编辑器。
                </n-text>
              </div>

              <div>
                <n-text strong>3. 任务与分支关联</n-text>
                <n-text depth="3" style="display: block; margin-top: 8px">
                  创建任务时关联对应的分支，方便追踪每个分支的开发进度。
                  可以在任务描述中添加相关的 commit hash 或 PR 链接。
                </n-text>
              </div>

              <div>
                <n-text strong>4. 利用任务优先级</n-text>
                <n-text depth="3" style="display: block; margin-top: 8px">
                  使用优先级（高/中/低）标记任务紧急程度，高优先级任务会用红色标识。
                  在「进行中」列中优先处理高优先级任务。
                </n-text>
              </div>

              <div>
                <n-text strong>5. 终端复用</n-text>
                <n-text depth="3" style="display: block; margin-top: 8px">
                  为不同工作区打开独立的终端标签页，避免频繁 cd 切换目录。
                  可以在一个终端运行开发服务器，另一个执行 Git 命令。
                </n-text>
              </div>

              <div>
                <n-text strong>6. 笔记本的妙用</n-text>
                <n-text depth="3" style="display: block; margin-top: 8px">
                  在笔记本中记录常用的项目命令（如构建、测试、部署命令），
                  需要时直接复制到终端执行。还可以记录环境配置、API 文档链接等。
                </n-text>
              </div>

              <div>
                <n-text strong>7. 个性化主题</n-text>
                <n-text depth="3" style="display: block; margin-top: 8px">
                  在设置中调整主题颜色，打造符合个人审美的界面。
                  可以设置暗色主题保护视力。
                </n-text>
              </div>
            </n-space>
          </div>

          <n-divider />

          <div>
            <n-h3>
              <n-icon size="20"><WarningOutline /></n-icon>
              常见问题与注意事项
            </n-h3>
            <n-space vertical>
              <n-alert type="warning" title="工作区路径问题">
                工作区创建在项目根目录下的 <n-text code>.worktrees/</n-text> 目录中。
                建议在 .gitignore 中添加此目录，避免提交到仓库。
              </n-alert>

              <n-alert type="info" title="终端编码问题">
                Windows 系统默认使用 GBK 编码，可能导致中文乱码。
                升级到 PowerShell 7 可以部分解决这个问题。
              </n-alert>

              <n-alert type="warning" title="磁盘空间">
                每个工作区都会占用磁盘空间（包含完整的项目文件）。
                定期清理不再使用的工作区释放空间。
              </n-alert>

              <n-alert type="info" title="数据存储">
                任务、工作区信息等数据存储在应用数据库中。
                删除项目只会移除记录，不会影响 Git 仓库本身。
              </n-alert>
            </n-space>
          </div>

          <n-divider />

          <div>
            <n-h3>
              <n-icon size="20"><CodeOutline /></n-icon>
              典型工作流示例
            </n-h3>
            <n-card>
              <n-steps vertical :current="null">
                <n-step title="接到新功能需求">
                  <n-text depth="3">在看板的「待办」列创建任务，填写功能描述和验收标准。</n-text>
                </n-step>

                <n-step title="开始开发">
                  <n-space vertical size="small">
                    <n-text depth="3">
                      从 develop 分支创建新的功能分支 feature/xxx，并为其创建工作区。
                      将任务卡片拖到「进行中」列。
                    </n-text>
                  </n-space>
                </n-step>

                <n-step title="编码实现">
                  <n-space vertical size="small">
                    <n-text depth="3">
                      点击工作区的「在编辑器中打开」，开始编写代码。
                    </n-text>
                    <n-text depth="3">
                      在终端中运行 CLI AI 编程工具（如 claude code），或运行测试、启动开发服务器等。
                    </n-text>
                  </n-space>
                </n-step>

                <n-step title="提交代码">
                  <n-space vertical size="small">
                    <n-text depth="3">
                      在终端中执行 git add、git commit、git push 等命令。
                    </n-text>
                    <n-text depth="3">
                      可以在任务描述中记录关键的 commit hash。
                    </n-text>
                  </n-space>
                </n-step>

                <n-step title="从主分支 Rebase">
                  <n-space vertical size="small">
                    <n-text depth="3">
                      开发过程中，在工作区的分支卡片上点击「Rebase」快捷按钮，选择要 rebase 的目标分支（通常是 develop 或 main）。
                    </n-text>
                    <n-text depth="3">
                      系统会自动执行 rebase 操作，如有冲突需要在终端中手动解决。
                    </n-text>
                    <n-text depth="3" type="info">
                      💡 也可以在终端手动执行：<n-text code>git fetch origin && git rebase origin/main</n-text>
                    </n-text>
                  </n-space>
                </n-step>

                <n-step title="代码评审（可选）">
                  <n-space vertical size="small">
                    <n-text depth="3">
                      如果团队使用 Pull Request 流程，提交 PR 后，在笔记本中记录 PR 链接。
                    </n-text>
                    <n-text depth="3">
                      如需修改，继续在该工作区中开发。
                    </n-text>
                  </n-space>
                </n-step>

                <n-step title="合并代码回主分支">
                  <n-space vertical size="small">
                    <n-text depth="3">
                      功能开发完成后，在工作区卡片上点击「合并至」按钮，选择目标分支（主分支）。
                    </n-text>
                    <n-text depth="3">
                      系统会自动将当前分支的代码合并到目标分支，并推送到远程仓库。
                    </n-text>
                    <n-text depth="3" type="info">
                      💡 也可以切换到主分支工作区，在终端手动执行：<n-text code>git merge 功能分支名 && git push</n-text>
                    </n-text>
                  </n-space>
                </n-step>

                <n-step title="删除临时分支和工作区">
                  <n-space vertical size="small">
                    <n-text depth="3">
                      合并完成后，将任务卡片拖到「已完成」列。
                    </n-text>
                    <n-text depth="3">
                      在功能分支的工作区卡片菜单（点击卡片右上角的「···」）中选择「删除」，清理工作区目录。
                    </n-text>
                    <n-text depth="3">
                      系统会自动清理工作区目录和 Git worktree 记录。
                    </n-text>
                  </n-space>
                </n-step>
              </n-steps>
            </n-card>
          </div>
        </n-space>
      </n-card>

      <!-- 反馈与帮助 -->
      <n-card title="反馈与帮助">
        <n-space vertical size="large">
          <div>
            <n-text>一点说明</n-text>
            <n-ul>
              <n-li>此项目算是一个AI开发的练手项目，AI代码含量较高，只在选型阶段和后端的部分疑难问题上进行了人工干预。</n-li>
              <n-li>AI进化的速度超过了人们的想象，确实很厉害，顿时感觉自己处于一个夕阳行业中了。</n-li>
              <n-li>项目名字是临时乱起的，也许后面会有个正式点的名字和版本号。</n-li>
              <n-li>希望大家用来提高效率改善生活，不要用来内卷，搞窝里斗。</n-li>
            </n-ul>
          </div>

          <div>
            <n-text>
              如果你在使用过程中遇到问题，或有功能建议，欢迎通过以下方式反馈：
            </n-text>
            <n-ul>
              <n-li>
                <n-text>GitHub Issues：</n-text>
                <n-a href="https://github.com/fy0/aicode-kanban/issues" target="_blank">提交 bug 报告或功能请求</n-a>
              </n-li>
              <n-li>
                <n-text>项目仓库：</n-text>
                <n-a href="https://github.com/fy0/aicode-kanban" target="_blank">查看源码和文档</n-a>
              </n-li>
            </n-ul>
          </div>

          <n-divider />

          <div>
            <n-text strong style="font-size: 16px; display: block; margin-bottom: 16px">开发者</n-text>
            <n-space align="center">
              <n-avatar
                round
                :size="64"
                src="https://github.com/fy0.png"
                style="cursor: pointer"
                @click="openGithub"
              />
              <n-space vertical :size="4">
                <n-space :size="8">
                  <n-a href="https://github.com/fy0" target="_blank">
                    <n-space :size="4" align="center">
                      <n-text>@fy0</n-text>
                    </n-space>
                  </n-a>
                </n-space>
                <n-text depth="3" style="font-size: 13px">
                  感谢使用 AICode Kanban，希望能提升你的开发效率！
                </n-text>
              </n-space>
            </n-space>
          </div>

          <n-divider />

          <n-text type="success">
            ⭐ 如果觉得这个项目对你有帮助，欢迎在 GitHub 上给个 Star！
          </n-text>
        </n-space>
      </n-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router';
import { useTitle } from '@vueuse/core';
import {
  BookOutline,
  AlertCircleOutline,
  BulbOutline,
  RocketOutline,
  WarningOutline,
  CodeOutline,
} from '@vicons/ionicons5';
import { APP_NAME } from '@/constants/app';

useTitle(`使用指引 - ${APP_NAME}`);

const router = useRouter();

function goBack() {
  router.push({ name: 'projects' });
}

function openGithub() {
  window.open('https://github.com/fy0', '_blank');
}
</script>

<style scoped>
.user-guide-page {
  padding: 24px;
  max-width: 1200px;
  margin: 0 auto;
}

.guide-content {
  display: flex;
  flex-direction: column;
  gap: 24px;
  margin-top: 24px;
}

:deep(.n-card__content) {
  font-size: 14px;
  line-height: 1.8;
}

:deep(.n-h3) {
  margin-top: 16px;
  margin-bottom: 12px;
  display: flex;
  align-items: center;
  gap: 8px;
}

:deep(.n-h3:first-child) {
  margin-top: 0;
}

:deep(.n-ul),
:deep(.n-ol) {
  margin: 8px 0;
  padding-left: 24px;
}

:deep(.n-li) {
  margin: 4px 0;
}

:deep(.n-alert) {
  margin: 12px 0;
}

:deep(.n-steps) {
  margin-top: 16px;
}

:deep(.n-step) {
  padding-bottom: 24px;
}

:deep(.n-collapse) {
  margin-top: 16px;
}

:deep(.n-collapse-item__header) {
  font-size: 16px;
  font-weight: 500;
}
</style>
