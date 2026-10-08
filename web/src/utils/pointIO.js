const INTERNAL_FIELDS = ['_result', '_writeValue', '_quality']

const FIELD_ALIASES = {
  address: ['address', 'addr', 'point', 'tag', 'tagname', 'tag_name', 'plc_address'],
  offset: ['offset', 'offset_value', 'byte_offset', 'bit_offset', 'offs'],
  parseType: ['parseType', 'parsetype', 'dataType', 'datatype', 'type', 'data_type', 'parse_type'],
  expr: ['expr', 'expression', 'formula', 'calc']
}

function findValue(point, fieldName) {
  if (point[fieldName] !== undefined && point[fieldName] !== null) {
    return point[fieldName]
  }

  const aliases = FIELD_ALIASES[fieldName]
  if (aliases) {
    for (const alias of aliases) {
      if (point[alias] !== undefined && point[alias] !== null) {
        return point[alias]
      }
    }
  }

  return undefined
}

export function exportPoints(params, schemaColumns, filename = 'points.json') {
  if (!Array.isArray(params) || params.length === 0) {
    return false
  }

  const exportData = params.map(param => {
    const exported = { name: param.name || '' }
    schemaColumns.forEach(col => {
      if (col.name && col.name !== 'name' && !INTERNAL_FIELDS.includes(col.name)) {
        if (param[col.name] !== undefined && param[col.name] !== null) {
          exported[col.name] = param[col.name]
        }
      }
    })
    return exported
  })

  const blob = new Blob([JSON.stringify(exportData, null, 2)], {
    type: 'application/json;charset=utf-8'
  })

  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)

  return true
}

export function importPoints(file) {
  return new Promise((resolve, reject) => {
    if (!file || file.type !== 'application/json') {
      reject(new Error('请选择JSON格式文件'))
      return
    }

    const reader = new FileReader()
    reader.onload = (e) => {
      try {
        const content = e.target.result
        const data = JSON.parse(content)
        if (!Array.isArray(data)) {
          reject(new Error('文件格式错误，应为数组'))
          return
        }
        resolve(data)
      } catch (err) {
        reject(new Error('JSON解析失败'))
      }
    }
    reader.onerror = () => {
      reject(new Error('文件读取失败'))
    }
    reader.readAsText(file, 'utf-8')
  })
}

export function validateAndMapPoints(importedPoints, schemaColumns) {
  const mappedPoints = []
  const validFieldNames = new Set(['name'])

  schemaColumns.forEach(col => {
    if (col.name && !INTERNAL_FIELDS.includes(col.name)) {
      validFieldNames.add(col.name)
    }
  })

  const allKnownAliases = new Set()
  Object.values(FIELD_ALIASES).forEach(aliases => {
    aliases.forEach(alias => allKnownAliases.add(alias))
  })

  let warningMessages = []

  importedPoints.forEach((point, index) => {
    if (typeof point !== 'object' || point === null) {
      warningMessages.push(`第${index + 1}行数据格式不正确，已跳过`)
      return
    }

    const mapped = { name: point.name || '' }

    schemaColumns.forEach(col => {
      if (col.name && col.name !== 'name') {
        const value = findValue(point, col.name)
        if (value !== undefined) {
          mapped[col.name] = value
        } else if (col.default !== undefined) {
          mapped[col.name] = col.default
        } else if (col.type === 'int') {
          mapped[col.name] = 0
        } else if (col.type === 'select' && col.choices && col.choices.length > 0) {
          mapped[col.name] = col.choices[0]
        } else {
          mapped[col.name] = ''
        }
      }
    })

    const extraFields = Object.keys(point).filter(k => !validFieldNames.has(k) && !INTERNAL_FIELDS.includes(k))
    const unknownFields = extraFields.filter(f => !allKnownAliases.has(f))
    if (unknownFields.length > 0) {
      warningMessages.push(`第${index + 1}行存在不匹配的字段: ${unknownFields.join(', ')}，已忽略`)
    }

    mappedPoints.push(mapped)
  })

  return {
    points: mappedPoints,
    warnings: warningMessages
  }
}