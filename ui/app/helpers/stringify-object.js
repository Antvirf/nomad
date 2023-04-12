/**
 * Copyright (c) HashiCorp, Inc.
 * SPDX-License-Identifier: MPL-2.0
 */

import Helper from '@ember/component/helper';

/**
 * Changes a JSON object into a string
 */
export function stringifyObject(
  [obj],
  { replacer = null, whitespace = 2 } = {}
) {
  return JSON.stringify(obj, replacer, whitespace);
}

export default Helper.helper(stringifyObject);
