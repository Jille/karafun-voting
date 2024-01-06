<template>
  <q-layout view="lHh Lpr lFf">
    <q-header elevated>
      <q-toolbar>
        <q-toolbar-title class="row justify-between items-center">
          <template v-if="!searching">
            <router-link class="text-white" :to="{name: 'list', params: {list: 'theme'}}">Karafun</router-link>
            <q-space />
            <q-btn icon="search" @click="searching = true" round flat />
            <q-btn icon="person" round flat @click="askUsername=true" />
          </template>
          <template v-else>
            <q-btn icon="chevron_left" @click="searching = false" round flat />
            <q-input v-model="search" debounce="300" clearable color="white" dense autofocus class="col-grow" />
          </template>
        </q-toolbar-title>
      </q-toolbar>
    </q-header>

    <q-footer elevated>
      <q-expansion-item
          expand-separator
          v-model="showQueue"
          :label="queue.length == 0 ? 'Queue empty' : queue[0].song"
          :caption="queue.length == 0 ? '' : queue[0].artist"
          :icon="queue.length == 0 ? '' : 'music_note'"
          expand-icon="keyboard_arrow_up"
          v-if="queue.length > 0 || permissions.managePlayback"
        >
          <q-card>
            <q-card-section class="q-pa-none">
              <q-scroll-area style="height: 40vh;">
                <q-list bordered separator>
                  <q-item v-for="q in queue.slice(1)" :key="q.my_queue_id">
  <!--
                    <q-item-section avatar>
                      <q-avatar square>
                        <q-img src="https://cdnaws.recis.io/i/img/00/67/35/e9_bc69a1_sq200.jpg" fit="scale-down" />
                        <q-badge color="blue" floating>{{q.singers.length}}</q-badge>
                      </q-avatar>
                    </q-item-section>
  -->
                    <q-item-section>
                      <q-item-label class="text-primary">{{q.song}}</q-item-label>
                      <q-item-label caption>{{q.artist}}</q-item-label>
                      <q-item-label class="text-dark">
                        <template v-for="(s, idx) in q.singers" :key="idx"><template v-if="idx > 0">, </template>{{s}}</template>
                        <template v-if="q.singers.length < q.min_singers">, but looking for {{q.min_singers - q.singers.length}} more singer<template v-if="q.min_singers - q.singers.length != 1">s</template></template>
                      </q-item-label>
                    </q-item-section>
                    <q-item-section side>
                      <div class="q-gutter-sm" v-if="this.$q.screen.gt.sm">
                        <q-btn icon="delete" dense color="red" @click="remove(q.my_queue_id)" />
                        <q-btn v-if="q.singers.includes(username)" icon="keyboard_arrow_down" dense color="secondary" @click="moveDown(q.my_queue_id)" :disable="!q.can_move_down" />
                        <q-btn v-if="q.singers.includes(username)" icon="keyboard_arrow_up" dense color="secondary" @click="moveUp(q.my_queue_id)" :disable="!q.can_move_up" />
                        <q-btn v-if="!q.singers.includes(this.username)" icon="thumb_up" dense color="green" @click="upvote(q.my_queue_id)" />
                      </div>
                      <div v-else class="q-gutter-xs">
                        <q-btn icon="delete" dense color="red" @click="remove(q.my_queue_id)" />
                        <q-btn v-if="!q.singers.includes(this.username)" icon="thumb_up" dense color="green" @click="upvote(q.my_queue_id)" />
                        <br />
                        <q-btn v-if="q.singers.includes(username)" icon="keyboard_arrow_down" dense color="secondary" @click="moveDown(q.my_queue_id)" :disable="!q.can_move_down" />
                        <q-btn v-if="q.singers.includes(username)" icon="keyboard_arrow_up" dense color="secondary" @click="moveUp(q.my_queue_id)" :disable="!q.can_move_up" />
                      </div>
                    </q-item-section>
                  </q-item>
                </q-list>
              </q-scroll-area>

              <div v-if="permissions.managePlayback" class="q-pa-md">
                <q-toolbar class="q-gutter-md q-mt-sm q-px-none">
                  <q-btn v-if="status.playing" dense color="primary" icon="pause" @click="pause" />
                  <q-btn v-else dense color="primary" icon="play_arrow" @click="play" />
                  <q-btn dense color="primary" icon="skip_next" @click="next" />
                </q-toolbar>
                <div v-for="(t, idx) in status.tracks" :key="t.track_id">
                  <q-badge color="secondary">{{t.caption}}
                  </q-badge>
                  <q-slider v-model="t.volume" :min="0" :max="100" :step="1" v-if="permissions.manageVolumes" @update:model-value="setVolume(idx)"/>
                </div>
              </div>
            </q-card-section>
          </q-card>
        </q-expansion-item>
        <q-toolbar v-else>
          <q-toolbar-title>Queue empty</q-toolbar-title>
        </q-toolbar>
    </q-footer>

    <q-page-container>
      <router-view />
    </q-page-container>

    <q-inner-loading :showing="!connected">
      <q-spinner size="40%" color="primary" />
    </q-inner-loading>
  </q-layout>

  <q-dialog v-model="askUsername" :persistent="username == ''">
    <q-card class="q-pa-md">
      <q-card-section>
        <q-input v-model="username" label="Enter your name" @keyup.enter="setUsername" autofocus />
      </q-card-section>
      <q-card-actions align="right">
        <q-btn label="OK" flat :disable="username == ''" @click="setUsername" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts">
import { defineComponent, computed } from 'vue';

export default defineComponent({
  name: 'MainLayout',

  data() {
    return {
      connection: null as null|WebSocket,
      connected: false,
      username: '',
      askUsername: true,
      searching: false,
      search: '',
      queue: [],
      showQueue: false,
      permissions: {
        addToQueue: false,
        managePlayback: false,
        manageQueue: false,
        manageVolumes: false,
        viewQueue: false,
      },
      status: {
        playing: false,
        loading: false,
        tempo: 0,
        pitch: 0,
        tracks: [] as {
          track_id: number,
          volume: number,
          caption: string,
          color: string,
        }[],
      },
    }
  },

  mounted() {
    const username = this.$q.localStorage.getItem('username') as string|null
    if(username) {
      this.username = username
      this.askUsername = false
    }
    this.connect()
  },

  watch: {
    search(str: string) {
      if(!str) {
        return;
      }
      this.$router.replace({name: 'search', params: { search: str }});
    },
    '$route.path': {
      handler() {
        if(this.$q.screen.lt.sm) {
          this.showQueue = false;
        }
      },
    },
  },

  provide() {
    return {
      websocket: computed(() => this.connection),
      username: computed(() => this.username),
      permissions: computed(() => this.permissions),
    }
  },

  methods: {
    setUsername() {
      this.$q.localStorage.set('username', this.username)
      this.askUsername = false
    },
    connect() {
      this.connected = false
      this.connection = new WebSocket('ws://localhost:8066/ws');
      this.connection.onopen = () => {
        this.connection && this.connection.send(this.$route.params.channel as string);
        this.connected = true
      }
      this.connection.onclose = () => {
        this.connection = null
        this.connected = false
        setTimeout(() => this.connect(), 500)
      }
      this.connection.onmessage = (event) => {
        const p = JSON.parse(event.data)
        console.log(p)
        if(p.queue != null) {
          this.queue = p.queue
        }
        if(p.permissions != null) {
          this.permissions = p.permissions
        }
        if(p.status != null) {
          this.status = p.status
        }
      }
    },
    upvote(id: number) {
      const cmd = {
        command: 'upvote',
        my_queue_id: id,
        username: this.username,
      }
      this.connection && this.connection.send(JSON.stringify(cmd))
    },
    remove(id: number) {
      const cmd = {
        command: 'remove',
        my_queue_id: id,
      }
      this.connection && this.connection.send(JSON.stringify(cmd))
    },
    moveUp(id: number) {
      const cmd = {
        command: 'move-up',
        my_queue_id: id,
      }
      this.connection && this.connection.send(JSON.stringify(cmd))
    },
    moveDown(id: number) {
      const cmd = {
        command: 'move-down',
        my_queue_id: id,
      }
      this.connection && this.connection.send(JSON.stringify(cmd))
    },
    play() {
      const cmd = {
        command: 'play',
      }
      this.connection && this.connection.send(JSON.stringify(cmd))
    },
    pause() {
      const cmd = {
        command: 'pause',
      }
      this.connection && this.connection.send(JSON.stringify(cmd))
    },
    next() {
      const cmd = {
        command: 'next',
      }
      this.connection && this.connection.send(JSON.stringify(cmd))
    },
    setVolume(idx: number) {
      const cmd = {
        command: 'set-volume',
        track_id: this.status.tracks[idx].track_id,
        number: this.status.tracks[idx].volume,
      }
      this.connection && this.connection.send(JSON.stringify(cmd))
    }
  },
});
</script>
