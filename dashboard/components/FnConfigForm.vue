<template>
  <div class="form-group">
    <label class="col-sm-3 control-label">Config</label>
    <div class="col-sm-9">
      <div class="row" v-for="(line, index) in config" :key="index">
        <template v-if="!line.delete">
          <div class="col-sm-5 cfg-key">
            <!-- Don't allow the key to be edited if the form is being edited (providing it hasn't just been added (see issue #66)) -->
            <input type="text" class="form-control" placeholder="Key" v-model="line.key" :readonly="isEdit && !line.new"
              @keydown.enter.prevent="">
          </div>
          <div class="col-sm-5 cfg-val">
            <input type="text" class="form-control" placeholder="Value" v-model="line.value" @keydown.enter.prevent="">
          </div>
          <div class="col-sm-1 toolbar">
            <button class="btn btn-default" @click.prevent="removeConfigLine(index)"><i class="fa fa-times"></i></button>
          </div>
        </template>
      </div>
      <div>
        <a href="#" class="" @click.prevent="addConfigLine">
          <i class="fa fa-plus"></i> Add line
        </a>
      </div>
    </div>
  </div>
</template>

<script>
import { newConfig } from '../libs/util';

export default {
  props: ['config', 'isEdit'],
  methods: {
    addConfigLine: function () {
      this.config.push(newConfig());
    },
    removeConfigLine: function (index) {
      this.config[index].value = "";
      this.config[index].delete = true;
    },
  }
}
</script>

<style scoped>
.cfg-key {
  padding: 0 5px 5px 15px;
}

.cfg-val {
  padding: 0 5px 5px 5px;
  margin-right: -20px;
  width: 50%;
}
</style>
