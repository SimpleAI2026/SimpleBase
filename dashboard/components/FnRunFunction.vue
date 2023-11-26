<template>
  <modal title="Run Function" :show="show" @closed="closed" @ok="ok" @cancel="closed">
    <form class="form-horizontal" v-on:submit.prevent="ok">
      <div class="form-group">
        <label class="col-sm-3 control-label">App</label>
        <div class="col-sm-9">
          <input type="text" class="form-control" v-model="app.name" disabled>
        </div>
      </div>
      <div class="form-group">
        <label class="col-sm-3 control-label">Function Name</label>
        <div class="col-sm-9">
          <input type="text" class="form-control" v-model="fn.name" disabled>
        </div>
      </div>
      <div class="form-group">
        <label class="col-sm-3 control-label">Payload</label>
        <div class="col-sm-9">
          <textarea class="form-control" v-model="payload" placeholder="e.g. {}"></textarea>
        </div>
      </div>
      <div>
        <h5>cURL command</h5>
        <pre>curl -X POST -d '{{ payload }}' {{ apiUrl }}invoke/{{ fn.id }}</pre>
      </div>

      <div v-show="output">
        <h5>Output</h5>
        <pre>{{ output }}</pre>
      </div>
    </form>

    <div slot="footer">
      <button type="button" class="btn btn-primary" @click="ok" :disabled="submitting">Run</button>
    </div>
  </modal>
</template>

<script>
import Modal from '../libs/VueBootstrapModal.vue';
import { eventBus } from '../main';
import { defaultErrorHandler, getApiUrl, getAuthToken } from '../libs/util';

export default {
  props: ['app'],
  components: {
    Modal
  },
  data: function () {
    return {
      fn: {},
      show: false,
      submitting: false,
      payload: '{}',
      output: null,
      apiUrl: '',
    }
  },
  methods: {
    closed: function () {
      this.show = false;
    },
    ok: function () {
      var t = this;
      this.output = null;
      eventBus.$emit('NotificationClear');
      this.submitting = true;

      $.ajax({
        headers: { 'Authorization': getAuthToken() },
        url: '/api/fns/invoke/' + encodeURIComponent(this.fn.id),
        method: 'POST',
        data: JSON.stringify({ payload: this.payload }),
        contentType: "application/json",
        dataType: 'json',
        success: (res) => {
          console.log("res", res);
          t.submitting = false;
          t.output = res.output;
          // t.closed();
        },
        error: function (jqXHR, textStatus, errorThrown) {
          t.submitting = false;
          defaultErrorHandler(jqXHR);
        }
      })
    },
  },
  created: function () {
    var t = this;
    eventBus.$on('openRunFn', (fn) => {
      this.fn = fn;
      this.payload = '{}';
      this.output = null;
      this.show = true;
    });
    getApiUrl(url => t.apiUrl = url);
  }
}
</script>

<style scoped></style>
