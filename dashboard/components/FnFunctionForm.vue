<template>
  <modal :title="title()" :show="show" @closed="closed" @ok="ok" @cancel="closed">
    <form id="fnForm" class="form-horizontal" v-on:submit.prevent="ok">
      <div class="form-group">
        <label class="col-sm-3 control-label">Name *</label>
        <div class="col-sm-9">
          <input id="fnName" type="text" class="form-control" placeholder="e.g. hello" v-model="fn.name" :disabled="edit"
            @keydown.enter.prevent="">
        </div>
      </div>
      <input type="hidden" class="form-control" v-model="fn.app_id">
      <div class="form-group">
        <label class="col-sm-3 control-label">Image *</label>
        <div class="col-sm-9">
          <input id="fnImage" type="text" class="form-control" placeholder="e.g. fnproject/hello" v-model="fn.image"
            @keydown.enter.prevent="">
        </div>
      </div>
      <div class="form-group">
        <label class="col-sm-3 control-label">Memory, MB</label>
        <div class="col-sm-9">
          <input id="fnMemory" type="number" class="form-control" placeholder="e.g. 128" v-model.number="fn.memory"
            @keydown.enter.prevent="">
        </div>
      </div>
      <div class="form-group">
        <label class="col-sm-3 control-label">Timeout, sec</label>
        <div class="col-sm-9">
          <input id="fnTimeout" type="number" class="form-control" placeholder="e.g. 60" v-model.number="fn.timeout"
            @keydown.enter.prevent="">
        </div>
      </div>
      <div class="form-group">
        <label class="col-sm-3 control-label">Idle Timeout, sec</label>
        <div class="col-sm-9">
          <input id="FnIdleTimeout" type="number" class="form-control" placeholder="e.g. 60"
            v-model.number="fn.idle_timeout" @keydown.enter.prevent="">
        </div>
      </div>

      <hr>
      <fn-config-form :config="fnConfig" :isEdit="edit"></fn-config-form>

    </form>

    <div slot="footer">
      <button id="submitFn" type="button" class="btn btn-primary" @click="ok" :disabled="submitting">{{ okBtnName()
      }}</button>
    </div>
  </modal>
</template>

<script>
import Modal from '../libs/VueBootstrapModal.vue';
import FnConfigForm from '../components/FnConfigForm.vue';
import { eventBus } from '../main';
import {
  defaultErrorHandler, configToLines, linesToConfig, newConfig, getAuthToken
} from '../libs/util';

var defaultFn = function (app) {
  return jQuery.extend(true, {}, {
    name: "",
    app_id: app.id,
    image: "",
    memory: 128,
  });
}

export default {
  props: ['app'],
  components: {
    Modal,
    FnConfigForm,
  },
  data: function () {
    return {
      edit: false,
      show: false,
      submitting: false,
      fn: {},
      fnConfig: [],
    }
  },
  methods: {
    title: function () {
      return this.edit ? 'Edit Function' : 'Add New Function';
    },
    okBtnName: function () {
      return this.edit ? 'Save Changes' : 'Add Function';
    },
    closed: function () {
      this.show = false;
    },
    ok: function () {
      var t = this;
      eventBus.$emit('NotificationClear');
      this.submitting = true;

      this.fn.config = linesToConfig(this.fnConfig);

      if (this.edit) {
        var url = '/api/fns/' + encodeURIComponent(this.fn.id)
      } else {
        var url = '/api/fns/'
      }
      $.ajax({
        headers: { 'Authorization': getAuthToken() },
        url: url,
        method: this.edit ? 'PUT' : 'POST',
        data: JSON.stringify(this.fn),
        contentType: "application/json",
        dataType: 'json',
        success: (res) => {
          if (t.edit) {
            eventBus.$emit('FnUpdated', res.fn);
          } else {
            eventBus.$emit('FnAdded', res.fn);
          }
          t.submitting = false;
          t.closed();
        },
        error: function (jqXHR, textStatus, errorThrown) {
          t.submitting = false;
          defaultErrorHandler(jqXHR);
        }
      })
    },
  },
  created: function () {
    eventBus.$on('openEditFn', (fn) => {
      this.fn = jQuery.extend(true, {}, fn);
      this.fnConfig = configToLines(fn.config);
      this.edit = true;
      this.show = true;
    });
    eventBus.$on('openAddFn', () => {
      this.fn = defaultFn(this.app);
      this.fnConfig = [newConfig()];
      this.edit = false;
      this.show = true;
    });
  }
}
</script>
