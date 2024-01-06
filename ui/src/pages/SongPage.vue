<template>
  <q-spinner v-if="loading" />
  <q-page v-else class="row items-center justify-evenly">
    <img :src="song.img" />
    <div class="text-h5">{{song.artist.name}} - {{song.name}}</div>
    <div>
      <q-chip color="secondary" icon="calendar_month">{{song.year}}</q-chip>
      <q-chip color="secondary" icon="timer">{{duration}}</q-chip>
    </div>
    <div class="row items-center justify-evenly">
      <q-chip v-for="st in song.styles" :key="st.id" dense clickable @click="$router.push({name: 'songs', params: {filter: 'st_' + st.id}})">
       {{st.name}}
     </q-chip>
    </div>
    <q-btn label="Lyrics" @click="showLyrics = true" />

    <div v-if="false">
      <h2>Other songs from {{song.artist.name}}</h2>
      <SongsList :filter="'ar_' + song.artist.id +'_'+ song.id" />
    </div>

    <q-btn label="Add to queue" @click="queueDialog = true" v-if="permissions.addToQueue" />

    <q-dialog v-model="showLyrics">
      <q-card class="q-px-xl">
        <q-card-section style="white-space: pre-line">
          {{song.lyrics}}
        </q-card-section>
      </q-card>
    </q-dialog>

    <q-dialog v-model="queueDialog">
      <q-card class="q-px-xl">
        <q-card-section class="q-gutter-md">
          <div class="text-h4">{{ song.name }}</div>
          <q-input label="Singer" v-model="singerName" @keyup.enter="add" />
          <q-input label="Min singers" v-model.number="minSingers" type="number" @keyup.enter="add" />
          <q-btn label="Add to queue" color="primary" @click="add" />
        </q-card-section>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script lang="ts">
import { defineComponent, inject } from 'vue';
import SongsList from '../components/SongsList.vue';

export default defineComponent({
  name: 'SongsPage',
  components: { SongsList },
  props: {
    id: {
      type: String,
      required: true,
    },
  },
  data() {
    return {
      song: {duration: 0},
      loading: true,
      showLyrics: false,
      websocket: inject('websocket') as null|WebSocket,
      queueDialog: false,
      singerName: '',
      minSingers: 1,
    };
  },
  inject: [
    'username',
    'permissions'
  ],
  computed: {
    duration() {
      let secs = this.song.duration % 60 as string | number;
      if(secs < 10) {
        secs = '0' + secs;
      }
      return Math.round(this.song.duration / 60) +':'+ secs;
    }
  },
  watch: {
    id: {
      async handler(id: string) {
        this.loading = true;
        const resp = await this.$axios.get('https://www.karafun.co.uk/' + this.$route.params.channel + '?type=song_info&id='+ id);
        this.song = resp.data;
        this.loading = false;
      },
      immediate: true,
    },
    username: {
      handler(username: string) {
        this.singerName = username;
      },
      immediate: true,
    },
  },
  methods: {
    add() {
      const cmd = {
        command: 'enqueue',
        song_id: +this.id,
        username: this.singerName,
        min_singers: this.minSingers,
      }
      this.websocket && this.websocket.send(JSON.stringify(cmd))
      this.queueDialog = false;
    }
  },
});
</script>
