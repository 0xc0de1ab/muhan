<script setup>
import { ref, onMounted, nextTick, watch } from 'vue'

// Reactive state
const serverUrl = ref('ws://127.0.0.1:4041')
const logs = ref([])
const commandInput = ref('')
const isConnected = ref(false)
const isConnecting = ref(false)
const terminalBody = ref(null)
const showSettings = ref(false)
const useScanlines = ref(true)

// Command history
const commandHistory = ref([])
const historyIndex = ref(-1)

// WebSocket instance
let socket = null

// ASCII welcome banner
const asciiBanner = `
   __  ___      __                  
  /  |/  /_ __ / /_  ___ ____  
 / /|_/ / // // _  \\/ _ \`/ _ \\ 
/_/  /_/\\_,_//_//_/\\_,_/_//_/ 
      M U H A N   M U D   C L I E N T
`

// Parse ANSI escape codes to HTML
const ansiToHtml = (text) => {
  let html = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')

  const colors = {
    '0': 'color: inherit; font-weight: normal; background-color: transparent;',
    '1': 'font-weight: bold;',
    '30': 'color: #1e1e24;', 
    '31': 'color: #ff6b6b;', 
    '32': 'color: #51cf66;', 
    '33': 'color: #fcc419;', 
    '34': 'color: #339af0;', 
    '35': 'color: #cc5de8;', 
    '36': 'color: #22b8cf;', 
    '37': 'color: #f1f3f5;', 
    '90': 'color: #868e96;', 
    '91': 'color: #ff8787;', 
    '92': 'color: #69db7c;', 
    '93': 'color: #ffd43b;', 
    '94': 'color: #4dabf7;', 
    '95': 'color: #da77f2;', 
    '96': 'color: #3bc9db;', 
    '97': 'color: #ffffff;', 
  }

  let openSpans = 0
  const regex = /\u001b\[([0-9;]*)m/g
  
  html = html.replace(regex, (match, p1) => {
    if (!p1 || p1 === '0') {
      let result = ''
      while (openSpans > 0) {
        result += '</span>'
        openSpans--
      }
      return result + '<span>'
    }
    
    let styles = ''
    const codes = p1.split(';')
    codes.forEach(code => {
      if (colors[code]) {
        styles += colors[code]
      }
    })
    
    openSpans++
    return `<span style="${styles}">`
  })

  while (openSpans > 0) {
    html += '</span>'
    openSpans--
  }
  
  return html
}

// Append logs
const addLog = (message, isInput = false) => {
  if (!logs.value) logs.value = []
  if (isInput) {
    logs.value.push({ text: `\n> ${message}`, isInput: true })
  } else {
    logs.value.push({ text: message, isInput: false })
  }
  
  // Cap logs at 1000 lines to prevent memory leak
  if (logs.value.length > 1000) {
    logs.value.shift()
  }

  scrollToBottom()
}

// Auto scroll
const scrollToBottom = () => {
  nextTick(() => {
    if (terminalBody.value) {
      terminalBody.value.scrollTop = terminalBody.value.scrollHeight
    }
  })
}

// Initialize connection
const connect = () => {
  if (socket) {
    socket.close()
  }

  isConnecting.value = true
  isConnected.value = false
  addLog('\n서버에 연결을 시도하는 중입니다...')

  try {
    socket = new WebSocket(serverUrl.value)

    socket.onopen = () => {
      isConnected.value = true
      isConnecting.value = false
      addLog('\n서버에 성공적으로 연결되었습니다.\n')
    }

    socket.onmessage = (event) => {
      addLog(event.data)
    }

    socket.onclose = () => {
      isConnected.value = false
      isConnecting.value = false
      addLog('\n서버와의 연결이 끊어졌습니다.\n')
    }

    socket.onerror = (error) => {
      isConnecting.value = false
      addLog(`\n오류가 발생했습니다: ${error.message || '서버에 연결할 수 없습니다.'}\n`)
    }
  } catch (e) {
    isConnecting.value = false
    addLog(`\n연결 예외 발생: ${e.message}\n`)
  }
}

// Send command
const sendCommand = (cmdText) => {
  const text = cmdText || commandInput.value
  if (!text.trim()) return

  if (!cmdText) {
    commandInput.value = ''
  }

  // Push to history
  commandHistory.value.push(text)
  historyIndex.value = commandHistory.value.length

  if (socket && isConnected.value) {
    // Show local echo (optional, but standard for MUD is server echoing. 
    // Let's write the command to the log directly for prompt feedback).
    addLog(text, true)
    socket.send(text + '\n')
  } else {
    addLog(text, true)
    addLog('\n[오류] 서버에 연결되어 있지 않아 명령어를 보낼 수 없습니다.\n')
  }
}

// History cycling (Arrow Keys)
const cycleHistory = (direction) => {
  if (commandHistory.value.length === 0) return

  if (direction === 'up') {
    if (historyIndex.value > 0) {
      historyIndex.value--
      commandInput.value = commandHistory.value[historyIndex.value]
    }
  } else if (direction === 'down') {
    if (historyIndex.value < commandHistory.value.length - 1) {
      historyIndex.value++
      commandInput.value = commandHistory.value[historyIndex.value]
    } else {
      historyIndex.value = commandHistory.value.length
      commandInput.value = ''
    }
  }
}

// Quick command buttons helper
const quickCmd = (cmd) => {
  sendCommand(cmd)
}

// Clear screen
const clearScreen = () => {
  logs.value = []
}

// Set WebSocket host automatically on mount
onMounted(() => {
  const hostname = window.location.hostname || '127.0.0.1'
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const port = window.location.port

  // Docker/Nginx 환경: 같은 호스트의 /ws 경로 사용
  // 로컬 개발 환경 (Vite :5173): 직접 :4041 포트 연결
  if (port === '80' || port === '443' || port === '' || port === '8080') {
    // 프로덕션/Docker 환경 (Nginx 프록시)
    serverUrl.value = `${protocol}//${hostname}${port && port !== '80' && port !== '443' ? ':' + port : ''}/ws`
  } else {
    // 로컬 개발 환경
    serverUrl.value = `ws://${hostname}:4041`
  }
  connect()
})
</script>

<template>
  <div :class="{ 'scanlines': useScanlines }"></div>

  <!-- Header -->
  <header class="app-header">
    <div class="logo-container">
      <div class="logo-text">MUHAN MUD</div>
      <div class="logo-badge">Go Port</div>
    </div>

    <div style="display: flex; gap: 12px; align-items: center;">
      <!-- Connection Badge -->
      <div 
        class="status-badge" 
        :class="{ 
          'connected': isConnected, 
          'connecting': isConnecting, 
          'disconnected': !isConnected && !isConnecting 
        }"
      >
        <span class="status-dot"></span>
        <span>
          {{ isConnected ? '연결 완료' : isConnecting ? '연결 중...' : '연결 끊김' }}
        </span>
      </div>

      <!-- Settings Button -->
      <button class="console-btn" @click="showSettings = true" title="설정">
        <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="3"></circle>
          <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
        </svg>
      </button>
    </div>
  </header>

  <!-- Main Grid -->
  <main class="main-layout">
    <!-- Left Column: Terminal -->
    <section class="terminal-panel glass-panel">
      <!-- Terminal Window Header -->
      <div class="terminal-header">
        <div class="terminal-title">
          <span>&gt;_ CONSOLE</span>
        </div>
        <div class="console-actions">
          <button class="console-btn" @click="clearScreen" title="화면 지우기">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
            </svg>
          </button>
        </div>
      </div>

      <!-- Console Body -->
      <div class="terminal-body" ref="terminalBody">
        <!-- ASCII Welcome Art -->
        <pre class="ascii-welcome">{{ asciiBanner }}</pre>

        <!-- Log List -->
        <template v-for="(log, idx) in logs" :key="idx">
          <div v-if="log.isInput" style="color: var(--color-primary); font-weight: bold;">
            {{ log.text }}
          </div>
          <div v-else v-html="ansiToHtml(log.text)"></div>
        </template>
      </div>

      <!-- Command Input -->
      <div class="terminal-input-container">
        <div class="input-prompt">&gt;</div>
        <input 
          type="text" 
          class="terminal-input" 
          v-model="commandInput"
          @keydown.enter="sendCommand()"
          @keydown.up.prevent="cycleHistory('up')"
          @keydown.down.prevent="cycleHistory('down')"
          placeholder="명령어를 입력하세요..." 
          autofocus
        />
      </div>
    </section>

    <!-- Right Column: Sidebar controls -->
    <aside class="sidebar-panel glass-panel">
      <div class="sidebar-content">
        <!-- Quick Action Keypad -->
        <div class="sidebar-section">
          <div class="section-title">방향 이동 키패드</div>
          <div class="keypad-grid">
            <button class="keypad-btn" @click="quickCmd('위')">위</button>
            <button class="keypad-btn" @click="quickCmd('북')">북</button>
            <button class="keypad-btn empty"></button>
            
            <button class="keypad-btn" @click="quickCmd('서')">서</button>
            <button class="keypad-btn" @click="quickCmd('봐')">봐</button>
            <button class="keypad-btn" @click="quickCmd('동')">동</button>
            
            <button class="keypad-btn" @click="quickCmd('밑')">밑</button>
            <button class="keypad-btn" @click="quickCmd('남')">남</button>
            <button class="keypad-btn empty"></button>
          </div>
        </div>

        <!-- Quick Action Command Buttons -->
        <div class="sidebar-section">
          <div class="section-title">빠른 명령어</div>
          <div class="action-grid">
            <button class="action-btn" @click="quickCmd('상태')">상태</button>
            <button class="action-btn" @click="quickCmd('인벤')">인벤토리</button>
            <button class="action-btn" @click="quickCmd('도망')">도망</button>
            <button class="action-btn" @click="quickCmd('저장')">게임 저장</button>
            <button class="action-btn" @click="quickCmd('목매달기')" style="border-color: rgba(231, 76, 60, 0.3); color: var(--color-danger);">자살신청</button>
            <button class="action-btn" @click="quickCmd('접속자')">접속자</button>
          </div>
        </div>

        <!-- Info Card -->
        <div class="sidebar-section">
          <div class="section-title">게임 인포메이션</div>
          <div class="game-stats-grid">
            <div class="stat-card">
              <div class="stat-val">인제로</div>
              <div class="stat-lbl">Default Character</div>
            </div>
            <div class="stat-card">
              <div class="stat-val">127.0.0.1</div>
              <div class="stat-lbl">Server IP</div>
            </div>
          </div>
        </div>

        <!-- Extra Display Toggle Settings -->
        <div class="sidebar-section">
          <div class="section-title">디스플레이 필터</div>
          <div style="display: flex; align-items: center; justify-content: space-between; font-size: 13px;">
            <span>CRT 스캔라인 효과</span>
            <input type="checkbox" v-model="useScanlines" style="cursor: pointer;" />
          </div>
        </div>
      </div>
    </aside>
  </main>

  <!-- Settings Modal Overlay -->
  <div class="settings-overlay" v-if="showSettings">
    <div class="settings-modal glass-panel">
      <div class="modal-header">
        <div class="modal-title">웹소켓 서버 설정</div>
        <button class="console-btn" @click="showSettings = false">
          <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <line x1="18" y1="6" x2="6" y2="18"></line>
            <line x1="6" y1="6" x2="18" y2="18"></line>
          </svg>
        </button>
      </div>
      <div class="form-group">
        <label for="ws-url">웹소켓 서버 주소</label>
        <input 
          type="text" 
          id="ws-url" 
          class="form-control" 
          v-model="serverUrl" 
          placeholder="ws://localhost:4041"
          @keyup.enter="connect(); showSettings = false"
        />
      </div>
      <button class="btn-primary" @click="connect(); showSettings = false" style="width: 100%;">
        서버 재연결
      </button>
    </div>
  </div>
</template>
