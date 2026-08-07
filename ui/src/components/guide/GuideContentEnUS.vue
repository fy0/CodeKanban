<template>
  <div class="guide-content">
    <!-- Welcome Introduction -->
    <n-card title="Welcome to Code Kanban">
      <n-space vertical size="large">
        <n-text>
          Code Kanban is an efficiency tool designed for the
          <n-text strong>AI programming era</n-text>, specifically created to solve the pain points
          of managing multiple projects and terminals with CLI AI programming tools.
        </n-text>

        <n-text>
          If you're like me, frequently switching between 3-4 code repositories daily, with each
          project requiring 5+ terminals running different AI programming tools, you'll appreciate
          this tool.
        </n-text>

        <n-alert type="warning" title="Challenges in the AI Programming Era">
          When using CLI AI programming tools like Claude Code, Codex, Qwen Code, you often need to
          switch between multiple projects and branches frequently. Traditional terminal windows
          become chaotic, ongoing tasks are hard to track, and running multiple AI CLI tools
          simultaneously creates even more confusion. Code Kanban helps you control everything in
          one interface!
        </n-alert>

        <n-alert type="success" title="Core Advantages">
          <n-space vertical size="small">
            <div>
              <n-text strong>🚀 Quick Multi-Project Switching</n-text>
              <n-text depth="3" style="display: block; margin-top: 4px">
                Manage all projects in one interface, switch to any worktree of any project anytime,
                without closing editors or terminals. Say goodbye to tedious directory switching and
                project searching.
              </n-text>
            </div>
            <n-divider style="margin: 8px 0" />
            <div>
              <n-text strong>💻 Centralized Terminal Management</n-text>
              <n-text depth="3" style="display: block; margin-top: 4px">
                All project terminal sessions are centralized in one interface with multi-tab
                management. Open independent terminals for different projects and worktrees, switch
                with one click, say goodbye to chaotic terminal windows.
              </n-text>
            </div>
          </n-space>
        </n-alert>

        <n-alert type="info" title="Powered by Git Worktree">
          Using Git Worktree technology, seamlessly switch between different branches of the same
          project, without frequent checkout and stash operations. Each worktree independently
          maintains its own file state.
        </n-alert>

        <n-alert type="info" title="Translation Notice">
          ℹ️ This is an automatically translated version. Some terminology may not be perfect. The
          Chinese version is the authoritative source.
        </n-alert>
      </n-space>
    </n-card>

    <!-- Workflow -->
    <n-card title="Workflow">
      <n-tabs v-model:value="workflowTab" type="segment" animated>
        <n-tab-pane name="quick" tab="Quick Start (Suitable for Most Cases)">
          <n-steps vertical :current="null" style="margin-top: 16px">
            <n-step title="Add Project">
              <n-space vertical>
                <n-text depth="3">
                  Click the "Add Project" button in the top right corner of the project list page,
                  fill in the project name and path.
                </n-text>
                <n-text depth="3" type="info">
                  <n-icon size="16"><AlertCircleOutline /></n-icon>
                  It's recommended to use the Git repository root directory, otherwise Worktree
                  features won't be available.
                </n-text>
              </n-space>
            </n-step>

            <n-step title="Start Terminal and Begin Work">
              <n-space vertical>
                <n-text depth="3">
                  Click the "Open Terminal" button on the Worktree card to automatically open a
                  terminal and switch to that worktree directory.
                </n-text>
                <n-text depth="3">
                  Now you can open multiple terminals based on your tasks, run several AI
                  programming tools in them, and it's best to rename the terminal titles to the task
                  names to avoid confusion.
                </n-text>
                <n-text depth="3">
                  The terminal window shortcut is ` (the key to the left of number 1), which can be
                  used to toggle the terminal when focus is not on it.
                </n-text>
                <n-text depth="3" type="success">
                  <n-icon size="16"><AlertCircleOutline /></n-icon>
                  The terminal is suitable for running CLI AI programming tools (like claude code,
                  codex, droid, gemini code, etc.), centrally managing all AI programming sessions
                  in one interface, switching projects and worktrees at any time.
                </n-text>
              </n-space>
            </n-step>

            <n-step title="Complete Tasks">
              <n-space vertical>
                <n-text depth="3"> Commit code when you feel appropriate. </n-text>
                <n-text depth="3">
                  Additionally, there's a simple task board for goal management and a notepad for
                  quick notes (right side of screen, shortcut key 1). By the way, terminals can be
                  launched from the task board, and the terminal title will directly use the
                  corresponding task title.
                </n-text>
                <n-text depth="3">
                  If you want AI to work on multiple things simultaneously without code conflicts,
                  you can use the worktree management feature.
                </n-text>
              </n-space>
            </n-step>
          </n-steps>
        </n-tab-pane>

        <n-tab-pane name="standard" tab="More Standard Process (Complex Workflow Using Worktree)">
          <n-steps vertical :current="null" style="margin-top: 16px">
            <n-step title="Add Project">
              <n-space vertical>
                <n-text depth="3">
                  Click the "Add Project" button in the top right corner of the project list page,
                  fill in the project name and path.
                </n-text>
                <n-text depth="3" type="info">
                  <n-icon size="16"><AlertCircleOutline /></n-icon>
                  It's recommended to use the Git repository root directory, otherwise Worktree
                  features won't be available.
                </n-text>
              </n-space>
            </n-step>

            <n-step title="Create Worktree">
              <n-space vertical>
                <n-text depth="3">
                  If the project is a Git repository, click the "Worktrees" tab, then click the
                  "New" button to create a worktree from a base branch.
                </n-text>
                <n-text depth="3">
                  The system will automatically create an independent worktree folder in the project
                  directory (using git worktree, located at ".worktrees/branch-name").
                </n-text>
                <n-text depth="3">
                  Some project workflows require creating a branch for each PR. You can use this
                  feature or switch branches directly in the terminal and refresh the page - both
                  work fine.
                </n-text>
              </n-space>
            </n-step>

            <n-step title="Manage Tasks">
              <n-space vertical>
                <n-text depth="3">
                  Create tasks on the kanban board. Drag task cards to adjust their status (To Do →
                  In Progress → Done).
                </n-text>
                <n-text depth="3">
                  Tasks can be associated with specific worktrees and branches.
                </n-text>
                <n-text depth="3"> List everything you need to do. </n-text>
              </n-space>
            </n-step>

            <n-step title="Start Terminal and Begin Work">
              <n-space vertical>
                <n-text depth="3">
                  Click the "Open Terminal" button on the Worktree card to automatically open a
                  terminal and switch to that worktree directory.
                </n-text>
                <n-text depth="3">
                  Now you can open multiple terminals based on your tasks, run several AI
                  programming tools in them, and it's best to rename the terminal titles to the task
                  names to avoid confusion.
                </n-text>
                <n-text depth="3">
                  The terminal window shortcut is ` (the key to the left of number 1), which can be
                  used to toggle the terminal when focus is not on it.
                </n-text>
                <n-text depth="3" type="success">
                  <n-icon size="16"><AlertCircleOutline /></n-icon>
                  The terminal is suitable for running CLI AI programming tools (like claude code,
                  codex, droid, gemini code, etc.), centrally managing all AI programming sessions
                  in one interface, switching projects and worktrees at any time.
                </n-text>
              </n-space>
            </n-step>

            <n-step title="Commit Code">
              <n-space vertical>
                <n-text depth="3"> Commit code when you feel appropriate. </n-text>
                <n-text depth="3">
                  If you're committing from a branch, keep the worktree clean before merging. The
                  system Git engine supports merge, rebase, and squash; the built-in engine supports
                  fast-forward merges only.
                </n-text>
              </n-space>
            </n-step>

            <n-step title="Complete Tasks">
              <n-space vertical>
                <n-text depth="3">
                  When everything is done, drag the task to "Done" on the task board.
                </n-text>
                <n-text depth="3"> Finally, delete the temporary working branch. Finished! </n-text>
              </n-space>
            </n-step>
          </n-steps>
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <!-- Core Features -->
    <n-card title="Core Features Explained">
      <n-collapse>
        <n-collapse-item title="Project Management" name="projects">
          <n-space vertical>
            <n-h3>Create Project</n-h3>
            <n-text
              >Click "Add Project" on the project list page and enter project information:</n-text
            >
            <n-ul>
              <n-li><strong>Project Name:</strong> Display name for easy identification</n-li>
              <n-li
                ><strong>Project Path:</strong> Absolute path to the project directory (Git
                repository root recommended)</n-li
              >
              <n-li
                ><strong>Default Branch:</strong> Usually main or master (only needed if project is
                a Git repository)</n-li
              >
              <n-li><strong>Description:</strong> Optional project description</n-li>
            </n-ul>
            <n-alert type="info" title="About Project Path">
              The project path doesn't strictly require a Git repository, but if it's not a Git
              repository, Worktree management, branch management, and other Git-related features
              won't be available.
            </n-alert>

            <n-h3>Edit Project</n-h3>
            <n-text
              >Click the more button (···) in the top right corner of the project card, select
              "Edit" to modify project metadata.</n-text
            >

            <n-h3>Delete Project</n-h3>
            <n-text depth="3" type="warning">
              <n-icon size="16"><AlertCircleOutline /></n-icon>
              Deleting a project only removes the database record and won't delete the Git
              repository on disk.
            </n-text>
          </n-space>
        </n-collapse-item>

        <n-collapse-item title="Worktree Management" name="worktrees">
          <n-space vertical>
            <n-alert type="warning" title="Prerequisites">
              The Worktree feature requires the project to be a valid Git repository. If the project
              path is not a Git repository, this feature will be unavailable.
            </n-alert>

            <n-h3>What is a Worktree?</n-h3>
            <n-text>
              Git Worktree allows you to check out multiple branches from the same repository into
              different directories simultaneously. This lets you quickly switch between different
              branches without needing to stash or commit unfinished work.
            </n-text>

            <n-h3>Create Worktree</n-h3>
            <n-ol>
              <n-li>Click "New Worktree"</n-li>
              <n-li>Select an existing branch or create a new branch</n-li>
              <n-li
                >The system will create a <n-text code>.worktrees/branch-name</n-text> directory in
                the project</n-li
              >
            </n-ol>

            <n-h3>Worktree Operations</n-h3>
            <n-ul>
              <n-li
                ><strong>Open in Editor:</strong> Opens the worktree directory using the configured
                editor (VS Code, Cursor, etc.)</n-li
              >
              <n-li
                ><strong>Open in Terminal:</strong> Opens a new terminal tab, automatically
                switching working directory to this worktree</n-li
              >
              <n-li
                ><strong>Delete Worktree:</strong> Deletes both the worktree directory and Git
                worktree record</n-li
              >
            </n-ul>

            <n-alert type="warning" title="Notes">
              <n-ul>
                <n-li
                  >The same branch cannot be checked out by multiple worktrees simultaneously</n-li
                >
                <n-li>Ensure important changes are committed before deleting a worktree</n-li>
              </n-ul>
            </n-alert>
          </n-space>
        </n-collapse-item>

        <n-collapse-item title="Task Kanban System" name="kanban">
          <n-space vertical>
            <n-h3>Kanban Layout</n-h3>
            <n-text>The kanban has three columns:</n-text>
            <n-ul>
              <n-li><strong>To Do:</strong> Planned tasks</n-li>
              <n-li><strong>In Progress:</strong> Tasks currently being developed</n-li>
              <n-li><strong>Done:</strong> Completed tasks</n-li>
            </n-ul>

            <n-h3>Create Task</n-h3>
            <n-text
              >Click the "+" button at the top of a column and fill in task information:</n-text
            >
            <n-ul>
              <n-li><strong>Title:</strong> Brief task description</n-li>
              <n-li
                ><strong>Description:</strong> Detailed task explanation (supports Markdown)</n-li
              >
              <n-li><strong>Priority:</strong> Low/Medium/High, indicated by colors</n-li>
              <n-li
                ><strong>Related Branch:</strong> Optional, marks which branch the task belongs
                to</n-li
              >
            </n-ul>

            <n-h3>Drag and Drop Sorting</n-h3>
            <n-text>You can:</n-text>
            <n-ul>
              <n-li
                >Drag task cards up and down within the same column to adjust priority order</n-li
              >
              <n-li>Drag task cards across columns to change task status</n-li>
            </n-ul>

            <n-h3>Task Operations</n-h3>
            <n-ul>
              <n-li>Click a task card to view details and full description</n-li>
              <n-li>Click the edit button to modify task information</n-li>
              <n-li>Click the delete button to remove a task</n-li>
            </n-ul>
          </n-space>
        </n-collapse-item>

        <n-collapse-item title="Integrated Terminal" name="terminal">
          <n-space vertical>
            <n-h3>Open Terminal</n-h3>
            <n-text
              >Click the "Open Terminal" button on a worktree card to automatically open the
              terminal panel and switch to that worktree's directory.</n-text
            >

            <n-h3>Collapse/Expand Terminal</n-h3>
            <n-text
              >Press the <n-text code>`</n-text> key (backtick) to quickly toggle the terminal
              panel's expand/collapse state, making it easy to view code or execute
              commands.</n-text
            >

            <n-h3>Multi-Tab Management</n-h3>
            <n-ul>
              <n-li
                >Click "+" on the right side of the terminal panel to create a new terminal
                tab</n-li
              >
              <n-li>Each tab is an independent Shell session</n-li>
              <n-li>Tabs display the current working directory name</n-li>
              <n-li>Click the "×" on the right side of a tab to close the terminal</n-li>
            </n-ul>

            <n-h3>Terminal Features</n-h3>
            <n-ul>
              <n-li>Supports full Shell commands (CMD, PowerShell, Bash, etc.)</n-li>
              <n-li>Supports color output and ANSI escape sequences</n-li>
              <n-li>Supports Ctrl+C to interrupt commands</n-li>
              <n-li>Supports copy and paste (right-click menu)</n-li>
              <n-li>Supports search (Ctrl+F)</n-li>
            </n-ul>

            <n-h3>Automatic Directory Switching</n-h3>
            <n-text
              >When clicking "Open Terminal" from a worktree card, the new terminal automatically
              switches to that worktree's directory.</n-text
            >
          </n-space>
        </n-collapse-item>

        <n-collapse-item title="Branch Management" name="branches">
          <n-space vertical>
            <n-h3>View Branches</n-h3>
            <n-text
              >Click "Branch Management" in the navigation bar at the top of the project page to
              view all local and remote branches.</n-text
            >

            <n-h3>Create Branch</n-h3>
            <n-text>Create new branches from existing branches for developing new features.</n-text>

            <n-h3>Switch Branches</n-h3>
            <n-text depth="3" type="warning">
              <n-icon size="16"><AlertCircleOutline /></n-icon>
              It's recommended to use different branches by creating worktrees rather than directly
              checking out.
            </n-text>
          </n-space>
        </n-collapse-item>

        <n-collapse-item title="Notepad" name="notepad">
          <n-space vertical>
            <n-h3>Quick Open</n-h3>
            <n-text
              >Press the <n-text code>1</n-text> key to quickly open/close the notepad panel</n-text
            >

            <n-h3>Project Notes</n-h3>
            <n-text>Each project has an independent notepad for recording:</n-text>
            <n-ul>
              <n-li>Project-related development notes</n-li>
              <n-li>Important commands or scripts</n-li>
              <n-li>Troubleshooting records</n-li>
              <n-li>To-do lists</n-li>
            </n-ul>

            <n-h3>Features</n-h3>
            <n-ul>
              <n-li>Supports Markdown format</n-li>
              <n-li>Real-time preview</n-li>
              <n-li>Auto-save</n-li>
            </n-ul>
          </n-space>
        </n-collapse-item>
      </n-collapse>
    </n-card>

    <!-- Usage Tips -->
    <n-card title="Usage Tips and Best Practices">
      <n-space vertical size="large">
        <div>
          <n-h3>
            <n-icon size="20"><BulbOutline /></n-icon>
            Efficient Multi-Branch Development Workflow
          </n-h3>
          <n-ol>
            <n-li>
              <strong>Main Development Branch:</strong> Create a permanent worktree for the main
              development branch (like develop)
            </n-li>
            <n-li>
              <strong>Feature Branches:</strong> Create independent branches and worktrees for each
              new feature
            </n-li>
            <n-li>
              <strong>Quick Switching:</strong> When you need to switch between different features,
              just open the corresponding worktree in your editor
            </n-li>
            <n-li>
              <strong>Emergency Fixes:</strong> When encountering urgent bugs, immediately create a
              worktree for the hotfix branch without interrupting current work
            </n-li>
          </n-ol>
        </div>

        <n-divider />

        <div>
          <n-h3>
            <n-icon size="20"><RocketOutline /></n-icon>
            Efficiency Tips
          </n-h3>
          <n-space vertical>
            <div>
              <n-text strong>1. Use Shortcuts for Efficiency</n-text>
              <n-text depth="3" style="display: block; margin-top: 8px">
                Press <n-text code>`</n-text> (backtick) to quickly expand/collapse the terminal
                panel, press <n-text code>1</n-text> to quickly open/close the notepad panel. This
                allows quick switching between work areas without leaving the keyboard.
              </n-text>
            </div>

            <div>
              <n-text strong>2. Customize Editor</n-text>
              <n-text depth="3" style="display: block; margin-top: 8px">
                Configure your preferred editor in settings (VS Code, Cursor, WebStorm, etc.), which
                will be automatically used when clicking "Open in Editor".
              </n-text>
            </div>

            <div>
              <n-text strong>3. Associate Tasks with Branches</n-text>
              <n-text depth="3" style="display: block; margin-top: 8px">
                Associate corresponding branches when creating tasks to easily track development
                progress for each branch. You can add related commit hashes or PR links in task
                descriptions.
              </n-text>
            </div>

            <div>
              <n-text strong>4. Utilize Task Priority</n-text>
              <n-text depth="3" style="display: block; margin-top: 8px">
                Use priority levels (High/Medium/Low) to mark task urgency. High-priority tasks are
                marked in red. Prioritize high-priority tasks in the "In Progress" column.
              </n-text>
            </div>

            <div>
              <n-text strong>5. Terminal Reuse</n-text>
              <n-text depth="3" style="display: block; margin-top: 8px">
                Open independent terminal tabs for different worktrees to avoid frequent cd
                directory switching. You can run a development server in one terminal and execute
                Git commands in another.
              </n-text>
            </div>

            <div>
              <n-text strong>6. Notepad Magic</n-text>
              <n-text depth="3" style="display: block; margin-top: 8px">
                Record frequently used project commands (like build, test, deploy commands) in the
                notepad, and copy them to the terminal when needed. You can also record environment
                configurations, API documentation links, etc.
              </n-text>
            </div>

            <div>
              <n-text strong>7. Personalized Theme</n-text>
              <n-text depth="3" style="display: block; margin-top: 8px">
                Adjust theme colors in settings to create an interface that suits your aesthetic.
                You can set a dark theme to protect your eyes.
              </n-text>
            </div>
          </n-space>
        </div>

        <n-divider />

        <div>
          <n-h3>
            <n-icon size="20"><WarningOutline /></n-icon>
            Common Issues and Notes
          </n-h3>
          <n-space vertical>
            <n-alert type="warning" title="Worktree Path Issue">
              Worktrees are created in the <n-text code>.worktrees/</n-text> directory under the
              project root. It's recommended to add this directory to .gitignore to avoid committing
              it to the repository.
            </n-alert>

            <n-alert type="info" title="Terminal Encoding Issue">
              Windows systems use GBK encoding by default, which may cause Chinese character
              garbling. Upgrading to PowerShell 7 can partially resolve this issue.
            </n-alert>

            <n-alert type="warning" title="Disk Space">
              Each worktree occupies disk space (contains complete project files). Regularly clean
              up unused worktrees to free up space.
            </n-alert>

            <n-alert type="info" title="Data Storage">
              Tasks, worktree information, and other data are stored in the application database.
              Deleting a project only removes records and doesn't affect the Git repository itself.
            </n-alert>
          </n-space>
        </div>

        <n-divider />

        <div>
          <n-h3>
            <n-icon size="20"><CodeOutline /></n-icon>
            Typical Workflow Example
          </n-h3>
          <n-card>
            <n-steps vertical :current="null">
              <n-step title="Receive New Feature Request">
                <n-text depth="3"
                  >Create a task in the "To Do" column on the kanban, fill in feature description
                  and acceptance criteria.</n-text
                >
              </n-step>

              <n-step title="Start Development">
                <n-space vertical size="small">
                  <n-text depth="3">
                    Create a new feature branch feature/xxx from the develop branch and create a
                    worktree for it. Drag the task card to the "In Progress" column.
                  </n-text>
                </n-space>
              </n-step>

              <n-step title="Code Implementation">
                <n-space vertical size="small">
                  <n-text depth="3">
                    Click "Open in Editor" on the worktree to start writing code.
                  </n-text>
                  <n-text depth="3">
                    Run CLI AI programming tools (like claude code) in the terminal, or run tests,
                    start development server, etc.
                  </n-text>
                </n-space>
              </n-step>

              <n-step title="Commit Code">
                <n-space vertical size="small">
                  <n-text depth="3">
                    Execute git add, git commit, git push commands in the terminal.
                  </n-text>
                  <n-text depth="3">
                    You can record key commit hashes in the task description.
                  </n-text>
                </n-space>
              </n-step>

              <n-step title="Prepare the Merge">
                <n-space vertical size="small">
                  <n-text depth="3">
                    Commit your work regularly and keep the worktree clean before preparing a merge.
                  </n-text>
                  <n-text depth="3">
                    Choose merge, rebase, or squash in Branch Management. With the built-in Git
                    engine, merge is limited to fast-forward; automatic writes prefer system Git.
                  </n-text>
                </n-space>
              </n-step>

              <n-step title="Code Review (Optional)">
                <n-space vertical size="small">
                  <n-text depth="3">
                    If the team uses Pull Request workflow, record the PR link in the notepad after
                    submitting the PR.
                  </n-text>
                  <n-text depth="3">
                    If modifications are needed, continue developing in that worktree.
                  </n-text>
                </n-space>
              </n-step>

              <n-step title="Merge Code Back to Main Branch">
                <n-space vertical size="small">
                  <n-text depth="3">
                    After feature development is complete, click the "Merge to" button on the
                    worktree card, select the target branch (main branch).
                  </n-text>
                  <n-text depth="3">
                    The system will automatically merge the current branch's code to the target
                    branch and push to the remote repository.
                  </n-text>
                  <n-text depth="3" type="info">
                    💡 Or switch to main branch worktree and manually execute in terminal:
                    <n-text code>git merge feature-branch-name && git push</n-text>
                  </n-text>
                </n-space>
              </n-step>

              <n-step title="Delete Temporary Branch and Worktree">
                <n-space vertical size="small">
                  <n-text depth="3">
                    After merging, drag the task card to the "Done" column.
                  </n-text>
                  <n-text depth="3">
                    Select "Delete" from the feature branch worktree card menu (click "···" in the
                    top right corner of the card) to clean up the worktree directory.
                  </n-text>
                  <n-text depth="3">
                    The system will automatically clean up the worktree directory and Git worktree
                    records.
                  </n-text>
                </n-space>
              </n-step>
            </n-steps>
          </n-card>
        </div>
      </n-space>
    </n-card>

    <!-- Feedback and Help -->
    <n-card title="Feedback and Help">
      <n-space vertical size="large">
        <div>
          <n-text>Some Notes</n-text>
          <n-ul>
            <n-li
              >This project is an AI development practice project with a high proportion of
              AI-generated code, with human intervention only during the technology selection phase
              and for some backend challenges.</n-li
            >
            <n-li
              >AI is evolving faster than people imagine. It's truly impressive, suddenly feels like
              being in a sunset industry.</n-li
            >
            <n-li
              >The project name was chosen randomly and temporarily. Perhaps there will be a more
              formal name and version number later.</n-li
            >
            <n-li
              >Hope everyone uses it to improve efficiency and enhance life, not for internal
              competition or infighting.</n-li
            >
            <n-li>If you find it useful, sponsorship is also welcome.</n-li>
          </n-ul>
        </div>

        <div>
          <n-text>
            If you encounter problems or have feature suggestions while using, feedback is welcome
            through the following channels:
          </n-text>
          <n-ul>
            <n-li>
              <n-text>GitHub Issues: </n-text>
              <a href="https://github.com/fy0/CodeKanban/issues" target="_blank">
                <n-text type="primary" strong>Submit bug reports or feature requests</n-text>
              </a>
            </n-li>
            <n-li>
              <n-text>Project Repository: </n-text>
              <a href="https://github.com/fy0/CodeKanban" target="_blank">
                <n-text type="primary" strong>View source code and documentation</n-text>
              </a>
            </n-li>
          </n-ul>
        </div>

        <n-divider />

        <div>
          <n-text strong style="font-size: 16px; display: block; margin-bottom: 16px"
            >Developer</n-text
          >
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
                <a href="https://github.com/fy0" target="_blank">
                  <n-space :size="4" align="center">
                    <n-text>@fy0</n-text>
                  </n-space>
                </a>
              </n-space>
              <n-text depth="3" style="font-size: 13px">
                Thank you for using Code Kanban, hope it boosts your development efficiency!
              </n-text>
            </n-space>
          </n-space>
        </div>

        <n-divider />

        <n-text type="success">
          <a href="https://github.com/fy0/CodeKanban" target="_blank">
            ⭐ If you find this project helpful, feel free to give it a Star on GitHub!
          </a>
        </n-text>
      </n-space>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import {
  AlertCircleOutline,
  BulbOutline,
  RocketOutline,
  WarningOutline,
  CodeOutline,
} from '@vicons/ionicons5';

const workflowTab = ref('quick');

function openGithub() {
  window.open('https://github.com/fy0', '_blank');
}
</script>

<style scoped>
.guide-content {
  display: flex;
  flex-direction: column;
  gap: 24px;
}
</style>
